package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Rule struct {
	ID                string `json:"id"`
	Description       string `json:"description"`
	Field             string `json:"field"`
	Operator          string `json:"operator"`
	Weight            int    `json:"weight"`
	Required          bool   `json:"required"`
	CompareWithDesign bool   `json:"compare_with_design"`
}

type Spec struct {
	VCPU     int     `json:"vcpu"`
	MemoryGB float64 `json:"memory_gb"`
	Label    string  `json:"label,omitempty"`
}

type Metrics struct {
	CPUUtilPct float64 `json:"cpu_util_pct"`
	MemUtilPct float64 `json:"mem_util_pct"`
}

type Workload struct {
	ConcurrentUsers int `json:"concurrent_users,omitempty"`
}

type Candidate struct {
	ID          string   `json:"id,omitempty"`
	Spec        Spec     `json:"spec"`
	Metrics     Metrics  `json:"metrics"`
	SimWorkload Workload `json:"sim_workload,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type DesignInput struct {
	PreferredVCPU     int      `json:"preferred_vcpu"`
	PreferredMemoryGB float64  `json:"preferred_memory_gb"`
	Workload          Workload `json:"workload,omitempty"`
}

type CandidateScore struct {
	Candidate        Candidate `json:"candidate"`
	PassedAllReq     bool      `json:"passed_all_required"`
	WorkloadDistance float64   `json:"workload_distance"`
	Suggestions      []string  `json:"suggestions"`
}

type Engine struct {
	rules []Rule
}

type RequestResponse struct {
	ID      string `json:"id"`
	Request struct {
		Design     DesignInput `json:"design"`
		Candidates []Candidate `json:"candidates"`
	} `json:"request"`
	Response []CandidateScore `json:"response"`
}

func NewEngineFromFile(path string) (*Engine, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules []Rule
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, err
	}
	return &Engine{rules: rules}, nil
}

func (e *Engine) EvaluateCandidates(design DesignInput, candidates []Candidate) ([]CandidateScore, error) {
	out := make([]CandidateScore, 0, len(candidates))
	for _, c := range candidates {
		cs, err := e.evalCandidate(design, c)
		if err != nil {
			return nil, err
		}
		cs.Suggestions = append(cs.Suggestions, GenerateSpecSuggestions(design, c)...)
		cs.Suggestions = append(cs.Suggestions, generateWorkloadSuggestions(design.Workload, c.SimWorkload, c.Metrics)...)
		out = append(out, cs)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PassedAllReq == out[j].PassedAllReq {
			return out[i].WorkloadDistance < out[j].WorkloadDistance
		}
		return out[i].PassedAllReq && !out[j].PassedAllReq
	})

	return out, nil
}

func (e *Engine) EvaluateAndStore(ctx context.Context, userID string, design DesignInput, candidates []Candidate) ([]CandidateScore, string, error) {
	out, err := e.EvaluateCandidates(design, candidates)
	if err != nil {
		return nil, "", err
	}
	if len(out) == 0 {
		return out, "", fmt.Errorf("no candidates evaluated")
	}
	best := out[0]

	dbURL := os.Getenv("DATABASE_URL")
	id := ""

	if dbURL != "" {
		pool, err := pgxpool.New(ctx, dbURL)
		if err == nil {
			defer pool.Close()
			id, err = saveRequestResponseToDB(ctx, pool, userID, design, candidates, out, best)
			if err == nil {
				return out, id, nil
			}
		}
	}
	u := uuid.New().String()
	rr := RequestResponse{
		ID: u,
	}
	rr.Request.Design = design
	rr.Request.Candidates = candidates
	rr.Response = out

	dir := filepath.Join("out", "asm", "req")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return out, "", fmt.Errorf("failed to create fallback dir: %v", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("request_response_%s.json", u))
	f, err := os.Create(path)
	if err != nil {
		return out, "", fmt.Errorf("failed to create fallback file: %v", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rr); err != nil {
		f.Close()
		return out, "", fmt.Errorf("failed to write fallback file: %v", err)
	}
	f.Close()
	return out, path, nil
}

func saveRequestResponseToDB(ctx context.Context, pool *pgxpool.Pool, userID string, design DesignInput, candidates []Candidate, response []CandidateScore, best CandidateScore) (string, error) {
	reqObj := struct {
		Design     DesignInput `json:"design"`
		Candidates []Candidate `json:"candidates"`
	}{
		Design:     design,
		Candidates: candidates,
	}
	reqJSON, err := json.Marshal(reqObj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request JSON: %v", err)
	}
	respJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response JSON: %v", err)
	}
	bestJSON, err := json.Marshal(best)
	if err != nil {
		return "", fmt.Errorf("failed to marshal best JSON: %v", err)
	}

	sql := `
INSERT INTO request_responses (user_id, request, response, best_candidate, created_at)
VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, now())
RETURNING id;
`
	var id string
	err = pool.QueryRow(ctx, sql, userID, string(reqJSON), string(respJSON), string(bestJSON)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("db insert failed: %v", err)
	}
	return id, nil
}

func (e *Engine) evalCandidate(design DesignInput, c Candidate) (CandidateScore, error) {
	passedAllReq := true

	for _, r := range e.rules {
		var val float64
		switch strings.ToLower(r.Field) {
		case "metrics.cpu_util_pct", "metrics.cpu_util", "cpu_util_pct":
			val = c.Metrics.CPUUtilPct
		case "metrics.mem_util_pct", "metrics.mem_util":
			val = c.Metrics.MemUtilPct
		case "spec.vcpu", "vcpu":
			val = float64(c.Spec.VCPU)
		case "spec.memory_gb", "memory_gb":
			val = c.Spec.MemoryGB
		case "design.preferred_vcpu":
			val = float64(design.PreferredVCPU)
		case "design.preferred_memory_gb":
			val = design.PreferredMemoryGB
		default:
			val = 0
		}

		if r.CompareWithDesign {
			if strings.Contains(strings.ToLower(r.Field), "vcpu") {
				val = float64(c.Spec.VCPU - design.PreferredVCPU)
			} else if strings.Contains(strings.ToLower(r.Field), "memory") {
				val = c.Spec.MemoryGB - design.PreferredMemoryGB
			}
		}

		if r.Required && val == 0 {
			passedAllReq = false
		}
	}

	dist := workloadDistance(design.Workload, c.SimWorkload)

	return CandidateScore{
		Candidate:        c,
		PassedAllReq:     passedAllReq,
		WorkloadDistance: dist,
		Suggestions:      nil,
	}, nil
}

func workloadDistance(target, sim Workload) float64 {
	du := float64(target.ConcurrentUsers - sim.ConcurrentUsers)
	return math.Abs(du)
}

func generateWorkloadSuggestions(target Workload, sim Workload, m Metrics) []string {
	s := []string{}
	if target.ConcurrentUsers == 0 || sim.ConcurrentUsers == 0 {
		return s
	}

	gap := target.ConcurrentUsers - sim.ConcurrentUsers

	if gap <= 0 {
		surplus := -gap
		surplusPercent := (float64(surplus) / float64(target.ConcurrentUsers)) * 100
		s = append(s, fmt.Sprintf(
			"Capacity exceeds target by %d users (%.0f%%). You may reduce resources to save cost.",
			surplus, surplusPercent))
		return s
	}

	shortfallPercent := (float64(gap) / float64(target.ConcurrentUsers)) * 100

	cpuBottleneck := m.CPUUtilPct > 75
	memBottleneck := m.MemUtilPct > 75
	bothBottlenecks := cpuBottleneck && memBottleneck
	lowUtilization := m.CPUUtilPct < 50 && m.MemUtilPct < 50

	if bothBottlenecks {
		s = append(s, fmt.Sprintf(
			"Shortfall of %d users (%.0f%%). Both CPU (%.1f%%) and memory (%.1f%%) are bottlenecked. Increase both vCPU and memory proportionally.",
			gap, shortfallPercent, m.CPUUtilPct, m.MemUtilPct))
	} else if cpuBottleneck {
		s = append(s, fmt.Sprintf(
			"Shortfall of %d users (%.0f%%). CPU is the limiting factor at %.1f%% utilization. Increase vCPU to improve throughput.",
			gap, shortfallPercent, m.CPUUtilPct))
	} else if memBottleneck {
		s = append(s, fmt.Sprintf(
			"Shortfall of %d users (%.0f%%). Memory is the limiting factor at %.1f%% utilization. Increase memory to improve capacity.",
			gap, shortfallPercent, m.MemUtilPct))
	} else if lowUtilization {
		s = append(s, fmt.Sprintf(
			"Shortfall of %d users (%.0f%%). Utilization is very low (CPU %.1f%%, memory %.1f%%), indicating an application-level bottleneck, not insufficient resources.",
			gap, shortfallPercent, m.CPUUtilPct, m.MemUtilPct))
	} else if m.CPUUtilPct > 60 || m.MemUtilPct > 60 {
		s = append(s, fmt.Sprintf(
			"Shortfall of %d users (%.0f%%). Moderate utilization (CPU %.1f%%, memory %.1f%%). Likely a mix of resource constraints and application limits. Try: Increase resources, Consider horizontal scaling.",
			gap, shortfallPercent, m.CPUUtilPct, m.MemUtilPct))
	}

	return s
}

func GenerateSpecSuggestions(design DesignInput, c Candidate) []string {
	s := []string{}

	cpuDesign := design.PreferredVCPU
	cpuCand := c.Spec.VCPU
	memDesign := design.PreferredMemoryGB
	memCand := c.Spec.MemoryGB

	cpuStressed := c.Metrics.CPUUtilPct > 75
	memStressed := c.Metrics.MemUtilPct > 75

	if cpuCand > cpuDesign {
		s = append(s, fmt.Sprintf("Increase vCPU from %d to %d", cpuDesign, cpuCand))
	} else if cpuCand < cpuDesign && !cpuStressed {
		s = append(s, fmt.Sprintf("Decrease vCPU from %d to %d", cpuDesign, cpuCand))
	} else if cpuCand < cpuDesign && cpuStressed {
		s = append(s, fmt.Sprintf("Keep vCPU at %d ", cpuCand))
	} else {
		s = append(s, fmt.Sprintf("Keep vCPU at %d", cpuDesign))
	}

	memDesignStr := formatMemory(memDesign)
	memCandStr := formatMemory(memCand)
	if memCand > memDesign {
		s = append(s, fmt.Sprintf("Increase memory from %s to %s", memDesignStr, memCandStr))
	} else if memCand < memDesign && !memStressed {
		s = append(s, fmt.Sprintf("Decrease memory from %s to %s", memDesignStr, memCandStr))
	} else if memCand < memDesign && memStressed {
		s = append(s, fmt.Sprintf("Keep memory at %s (insufficient for target workload)", memCandStr))
	} else {
		s = append(s, fmt.Sprintf("Keep memory at %s", memDesignStr))
	}

	if c.Metrics.CPUUtilPct > 90 {
		s = append(s, fmt.Sprintf("Critical: CPU utilization at %.1f%% — immediate scaling needed", c.Metrics.CPUUtilPct))
	} else if c.Metrics.CPUUtilPct < 20 {
		s = append(s, fmt.Sprintf("Low CPU utilization (%.1f%%) — candidate is significantly overprovisioned", c.Metrics.CPUUtilPct))
	}

	if c.Metrics.MemUtilPct > 90 {
		s = append(s, fmt.Sprintf("Critical: Memory utilization at %.1f%% — immediate scaling needed", c.Metrics.MemUtilPct))
	} else if c.Metrics.MemUtilPct < 20 {
		s = append(s, fmt.Sprintf("Low memory utilization (%.1f%%) — candidate may have excess memory", c.Metrics.MemUtilPct))
	}

	return s
}

func formatMemory(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d GB", int64(v))
	}
	return fmt.Sprintf("%.1f GB", v)
}