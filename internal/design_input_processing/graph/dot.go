package graph

import (
	"bytes"
	"fmt"
)

// ToDOT renders the graph as a GraphViz DOT file.
func ToDOT(g *Graph) []byte {
	var buf bytes.Buffer
	buf.WriteString("digraph G {\n")
	buf.WriteString("  rankdir=LR;\n")

	for _, n := range g.Nodes {
		label := n.Name
		if label == "" {
			label = n.ID
		}
		fmt.Fprintf(&buf, "  %q [label=%q];\n", n.ID, label)
	}

	for _, e := range g.Edges {
		lbl := e.Protocol
		if e.RPS > 0 {
			lbl = fmt.Sprintf("%s (%.0f rps)", e.Protocol, e.RPS)
		}
		fmt.Fprintf(&buf, "  %q -> %q [label=%q];\n", e.From, e.To, lbl)
	}

	buf.WriteString("}\n")
	return buf.Bytes()
}
