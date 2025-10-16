package rules

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"

// DetectGodService flags services with fan-out >= threshold.
func DetectGodService(g domain.Graph, threshold int) []Finding {
	if threshold <= 0 {
		threshold = 6
	}
	out := []Finding{}
	for svc, outs := range g.Adj {
		n := len(outs)
		if n >= threshold {
			sev := "medium"
			if n >= threshold*2 {
				sev = "high"
			}
			out = append(out, Finding{
				Kind:     "god_service",
				Severity: sev,
				Summary:  "service has very high fan-out (calls many others)",
				Nodes:    []string{svc},
				Meta:     map[string]any{"fan_out": n, "threshold": threshold},
			})
		}
	}
	return out
}
