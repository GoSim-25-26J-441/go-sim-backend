package rules

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
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

	requestResponse := RequestResponse{
		Response: out,
	}

	requestResponse.Request.Design = design
	requestResponse.Request.Candidates = candidates

	err := saveRequestResponseToFile(requestResponse, "request_response.json")
	if err != nil {
		return nil, err
	}

	return out, nil
}

func saveRequestResponseToFile(requestResponse RequestResponse, fileName string) error {
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("could not create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(requestResponse); err != nil {
		return fmt.Errorf("could not encode request-response to file: %v", err)
	}

	return nil
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
	if target.ConcurrentUsers > 0 && sim.ConcurrentUsers > 0 {
		if sim.ConcurrentUsers < target.ConcurrentUsers {
			s = append(s, fmt.Sprintf(
				"Simulated capacity supports ~%d concurrent users vs target %d; consider increasing vCPU or memory, or optimizing code paths to reach the target.",
				sim.ConcurrentUsers, target.ConcurrentUsers))
		} else if sim.ConcurrentUsers > target.ConcurrentUsers {
			s = append(s, fmt.Sprintf(
				"Simulated capacity exceeds target (supports ~%d users vs %d). You may reduce resources to save cost if utilization stays within limits.",
				sim.ConcurrentUsers, target.ConcurrentUsers))
		} else {
			s = append(s, "Simulated concurrent users match the target.")
		}
	}
	return s
}

func GenerateSpecSuggestions(design DesignInput, c Candidate) []string {
	s := []string{}

	cpuDesign := design.PreferredVCPU
	cpuCand := c.Spec.VCPU
	if cpuCand > cpuDesign {
		s = append(s, fmt.Sprintf("Increase vCPU from %d to %d", cpuDesign, cpuCand))
	} else if cpuCand < cpuDesign {
		s = append(s, fmt.Sprintf("Decrease vCPU from %d to %d", cpuDesign, cpuCand))
	} else {
		s = append(s, fmt.Sprintf("Keep vCPU at %d", cpuDesign))
	}

	memDesign := design.PreferredMemoryGB
	memCand := c.Spec.MemoryGB
	memDesignStr := formatMemory(memDesign)
	memCandStr := formatMemory(memCand)
	if memCand > memDesign {
		s = append(s, fmt.Sprintf("Increase memory from %s to %s", memDesignStr, memCandStr))
	} else if memCand < memDesign {
		s = append(s, fmt.Sprintf("Decrease memory from %s to %s", memDesignStr, memCandStr))
	} else {
		s = append(s, fmt.Sprintf("Keep memory at %s", memDesignStr))
	}

	if c.Metrics.CPUUtilPct > 90 {
		s = append(s, fmt.Sprintf("High CPU utilization (%.1f%%) — consider increasing vCPU further", c.Metrics.CPUUtilPct))
	} else if c.Metrics.CPUUtilPct < 30 {
		s = append(s, fmt.Sprintf("Low CPU utilization (%.1f%%) — candidate is overprovisioned", c.Metrics.CPUUtilPct))
	}
	if c.Metrics.MemUtilPct > 90 {
		s = append(s, fmt.Sprintf("High memory utilization (%.1f%%) — consider increasing memory", c.Metrics.MemUtilPct))
	} else if c.Metrics.MemUtilPct < 30 {
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
