package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SignalService handles signal extraction from chat history
type SignalService struct{}

// NewSignalService creates a new signal service
func NewSignalService() *SignalService {
	return &SignalService{}
}

// chatTurn represents a chat message (used for reading history)
type chatTurn struct {
	Role   string   `json:"role"`
	Text   string   `json:"text"`
	Ts     int64    `json:"ts"`
	Source string   `json:"source,omitempty"`
	Refs   []string `json:"refs,omitempty"`
	UserID string   `json:"user_id,omitempty"`
}

// Signal regexes
var rxRPS = regexp.MustCompile(`(?i)\b(\d+)\s*rps\b`)
var rxP95ms = regexp.MustCompile(`(?i)\bp95\b.*?(\d+)\s*ms`)
var rxCPU = regexp.MustCompile(`(?i)\b(\d+)\s*(v?cpu|cores?)\b`)
var rxPayload = regexp.MustCompile(`(?i)(\d+)\s*(kb|mb)\b`)
var rxBurst = regexp.MustCompile(`(?i)\b(\d+)[x×]\b`)

// LoadSignalsFromHistory loads signals from chat history for a job
func (s *SignalService) LoadSignalsFromHistory(jobID, userID string) map[string]int {
	recordSignalServiceCall()
	agg := map[string]int{}

	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	f, err := os.Open(fpath)
	if err != nil {
		return agg
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var t chatTurn
		if err := dec.Decode(&t); err != nil {
			if err == io.EOF {
				break
			}
			return agg
		}
		if t.Role != "user" {
			continue
		}
		signals := s.extractSignals(t.Text)
		for k, v := range signals {
			if v != 0 {
				agg[k] = v
			}
		}
	}
	return agg
}

// extractSignals extracts signals from a message
func (s *SignalService) extractSignals(msg string) map[string]int {
	out := map[string]int{}
	l := strings.ToLower(msg)

	rpsMatches := rxRPS.FindAllStringSubmatch(l, -1)
	if len(rpsMatches) >= 1 {
		out["rps_peak"] = atoiSafe(rpsMatches[0][1])
	}

	p95Matches := rxP95ms.FindAllStringSubmatch(l, -1)
	if len(p95Matches) >= 1 {
		out["latency_p95_ms"] = atoiSafe(p95Matches[0][1])
	}

	cpuMatches := rxCPU.FindAllStringSubmatch(l, -1)
	if len(cpuMatches) >= 1 {
		out["cpu_vcpu"] = atoiSafe(cpuMatches[0][1])
	}

	payloadMatches := rxPayload.FindAllStringSubmatch(l, -1)
	if len(payloadMatches) >= 1 {
		val := atoiSafe(payloadMatches[0][1])
		unit := strings.ToLower(payloadMatches[0][2])
		if unit == "mb" {
			val *= 1024
		}
		out["payload_kb"] = val
	}

	burstMatches := rxBurst.FindAllStringSubmatch(l, -1)
	if len(burstMatches) >= 1 {
		out["burst_factor"] = atoiSafe(burstMatches[0][1])
	}

	return out
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
