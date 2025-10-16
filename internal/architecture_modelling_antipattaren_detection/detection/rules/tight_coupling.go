package rules

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"

func DetectTightCoupling(g domain.Graph) []Finding {
	// mutual edges A→B and B→A
	hasEdge := map[string]map[string]bool{}
	for a, outs := range g.Adj {
		if hasEdge[a] == nil { hasEdge[a] = map[string]bool{} }
		for _, b := range outs { hasEdge[a][b] = true }
	}
	var out []Finding
	seen := map[string]bool{}
	for a, outs := range g.Adj {
		for _, b := range outs {
			if hasEdge[b][a] {
				key := a + "↔" + b
				rkey := b + "↔" + a
				if !seen[key] && !seen[rkey] {
					out = append(out, Finding{
						Kind:     "tight_coupling",
						Severity: "medium",
						Summary:  "mutual dependency between two services",
						Nodes:    []string{a, b},
					})
					seen[key], seen[rkey] = true, true
				}
			}
		}
	}
	return out
}
