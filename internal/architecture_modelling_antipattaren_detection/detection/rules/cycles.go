package rules

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"


func DetectCycles(g domain.Graph) []Finding {
	index := 0
	stack := []string{}
	onStack := map[string]bool{}
	idx := map[string]int{}
	low := map[string]int{}
	var out []Finding

	var strongConnect func(v string)
	strongConnect = func(v string) {
		idx[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range g.Adj[v] {
			if _, seen := idx[w]; !seen {
				strongConnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && idx[w] < low[v] {
				low[v] = idx[w]
			}
		}

		if low[v] == idx[v] {
			// start a new SCC
			var scc []string
			for {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[n] = false
				scc = append(scc, n)
				if n == v {
					break
				}
			}
			if len(scc) > 1 { // cycle only if SCC size > 1
				out = append(out, Finding{
					Kind:     "cycle",
					Severity: "high",
					Summary:  "cyclic dependency among services",
					Nodes:    scc,
				})
			}
		}
	}

	for _, n := range g.Nodes {
		if _, seen := idx[n]; !seen {
			strongConnect(n)
		}
	}
	return out
}
