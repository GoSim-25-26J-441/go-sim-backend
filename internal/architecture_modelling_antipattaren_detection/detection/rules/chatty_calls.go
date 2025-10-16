package rules

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"

// DetectChattyCalls flags edges with excessive per-minute call counts.

func DetectChattyCalls(s domain.Spec, threshold int) []Finding {
	if threshold <= 0 {
		threshold = 300
	}
	var out []Finding
	for from, spec := range s.Services {
		for to, freq := range spec.ChattyCalls {
			if freq >= threshold {
				sev := "medium"
				if freq >= threshold*2 {
					sev = "high"
				}
				out = append(out, Finding{
					Kind:     "chatty_calls",
					Severity: sev,
					Summary:  "excessive per-request or per-item call frequency",
					Nodes:    []string{from, to},
					Meta: map[string]any{
						"from": from, "to": to,
						"freq_per_min": freq, "threshold": threshold,
					},
				})
			}
		}
	}
	return out
}
