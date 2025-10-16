package export

import (
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

// ToDOT renders a simple Graphviz DOT for the dependency graph.
// Usage: dotBytes, _ := ToDOT(g); os.WriteFile("graph.dot", dotBytes, 0644)
func ToDOT(g domain.Graph) ([]byte, error) {
	var b strings.Builder
	b.WriteString("digraph G {\n")
	b.WriteString(`  rankdir=LR; node [shape=box, style="rounded,filled"];` + "\n")

	// Ensure nodes exist even if no edges
	for _, n := range g.Nodes {
		b.WriteString(`  "` + n + `";` + "\n")
	}
	for from, outs := range g.Adj {
		for _, to := range outs {
			b.WriteString(`  "` + from + `" -> "` + to + `";` + "\n")
		}
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}
