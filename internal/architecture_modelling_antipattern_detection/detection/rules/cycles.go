package rules

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type cycles struct{}
func (c cycles) Name() string { return "cycles" }

func (c cycles) Detect(g *domain.Graph) ([]domain.Detection, error) {
	// Tarjan SCC over CALLS edges between services
	index := 0
	stack := []string{}
	onStack := map[string]bool{}
	id := map[string]int{}
	low := map[string]int{}
	var dets []domain.Detection

	var nodes []string
	for _, n := range g.Nodes {
		if n.Kind == domain.NodeService { nodes = append(nodes, n.ID) }
	}
	var dfs func(v string)
	dfs = func(v string) {
		index++
		id[v], low[v] = index, index
		stack = append(stack, v); onStack[v] = true

		for _, e := range g.Out[v] {
			if e.Kind != domain.EdgeCalls { continue }
			w := e.To
			if _, seen := id[w]; !seen {
				dfs(w); if low[w] < low[v] { low[v] = low[w] }
			} else if onStack[w] && id[w] < low[v] {
				low[v] = id[w]
			}
		}
		// root?
		if low[v] == id[v] {
			comp := []string{}
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v { break }
			}
			if len(comp) > 1 {
				dets = append(dets, domain.Detection{
					Kind: domain.APCycles, Severity: domain.SeverityHigh,
					Title: "Cyclic dependency",
					Summary: "Services mutually depend on each other",
					Nodes: comp,
				})
			}
		}
	}
	for _, v := range nodes {
		if _, seen := id[v]; !seen { dfs(v) }
	}
	return dets, nil
}

func init(){ detection.Register(cycles{}) }
