package strategies

import (
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func nodeName(g *domain.Graph, nodeID string) string {
	if g != nil {
		if n, ok := g.Nodes[nodeID]; ok && n != nil && n.Name != "" {
			return n.Name
		}
	}
	parts := strings.SplitN(nodeID, ":", 2)
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}
	return nodeID
}

func findService(spec *parser.YSpec, name string) *parser.YService {
	for i := range spec.Services {
		if strings.EqualFold(spec.Services[i].Name, name) {
			return &spec.Services[i]
		}
	}
	return nil
}

func ensureService(spec *parser.YSpec, name string) *parser.YService {
	if s := findService(spec, name); s != nil {
		return s
	}
	spec.Services = append(spec.Services, parser.YService{
		Name: name,
	})
	return &spec.Services[len(spec.Services)-1]
}

func ensureDB(spec *parser.YSpec, dbName string) {
	for i := range spec.Databases {
		if strings.EqualFold(spec.Databases[i].Name, dbName) {
			return
		}
	}
	spec.Databases = append(spec.Databases, parser.YDatabase{Name: dbName})
}

func removeString(xs []string, v string) ([]string, bool) {
	out := make([]string, 0, len(xs))
	removed := false
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			removed = true
			continue
		}
		out = append(out, x)
	}
	return out, removed
}


func namesFromNodeIDs(g *domain.Graph, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, nodeName(g, id))
	}
	return out
}

func joinNice(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	if len(xs) == 1 {
		return xs[0]
	}
	if len(xs) == 2 {
		return xs[0] + " and " + xs[1]
	}

	var b strings.Builder
	for i := range xs {
		if i > 0 {
			if i == len(xs)-1 {
				b.WriteString(", and ")
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString(xs[i])
	}
	return b.String()
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		n := 0
		for i := 0; i < len(t); i++ {
			c := t[i]
			if c < '0' || c > '9' {
				if i == 0 {
					return 0, false
				}
				break
			}
			n = n*10 + int(c-'0')
		}
		return n, true
	default:
		return 0, false
	}
}


func findCallEdgeBetween(g *domain.Graph, fromName, toName string) (rpm int, endpoints int, ok bool) {
	if g == nil {
		return 0, 0, false
	}
	for _, e := range g.Edges {
		if e.Kind != domain.EdgeCalls {
			continue
		}
		if !strings.EqualFold(nodeName(g, e.From), fromName) {
			continue
		}
		if !strings.EqualFold(nodeName(g, e.To), toName) {
			continue
		}

		attrs := e.Attrs
		if attrs != nil {
			if r, ok2 := asInt(attrs["rate_per_min"]); ok2 {
				rpm = r
			}
			if eps, ok2 := attrs["endpoints"].([]string); ok2 {
				endpoints = len(eps)
			} else if epsAny, ok2 := attrs["endpoints"].([]any); ok2 {
				endpoints = len(epsAny)
			}
		}
		return rpm, endpoints, true
	}
	return 0, 0, false
}
