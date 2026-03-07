package export

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func ToDOT(g *domain.Graph, title string) string {
	var b strings.Builder
	b.WriteString("digraph G {\n  rankdir=LR;\n  node [shape=box, style=rounded];\n")
	if title != "" {
		b.WriteString(fmt.Sprintf(`  labelloc="t"; label="%s"; fontname="Helvetica";`, title))
		b.WriteString("\n")
	}

	for _, n := range g.Nodes {
		if n == nil {
			continue
		}
		style := `shape=box,style="rounded,filled",fillcolor="#eef6ff"`
		if n.Kind == domain.NodeDB {
			style = `shape=cylinder,style="filled",fillcolor="#fff3cd"`
		}
		b.WriteString(fmt.Sprintf(`  "%s" [label="%s", %s];`+"\n", n.ID, n.Name, style))
	}

	for i, e := range g.Edges {
		if e == nil {
			continue
		}

		lbl := string(e.Kind)

	
		if e.Kind == domain.EdgeCalls {
			
			if c, ok := e.Attrs["count"].(int); ok && c > 0 {
				lbl = fmt.Sprintf("calls (%d ep)", c)
			}
			if rpm, ok := e.Attrs["rate_per_min"].(int); ok && rpm > 0 {
				lbl = fmt.Sprintf("%s, %drpm", lbl, rpm)
			}

			
			if k, ok := e.Attrs["dep_kind"].(string); ok && k != "" {
				lbl = fmt.Sprintf("%s [%s]", lbl, k)
			}
			if s, ok := e.Attrs["sync"].(bool); ok {
				if s {
					lbl = fmt.Sprintf("%s (sync)", lbl)
				} else {
					lbl = fmt.Sprintf("%s (async)", lbl)
				}
			}
		}

		b.WriteString(fmt.Sprintf(`  "%s" -> "%s" [label="%s", tooltip="edge#%d"];`+"\n",
			e.From, e.To, lbl, i))
	}

	b.WriteString("}\n")
	return b.String()
}
