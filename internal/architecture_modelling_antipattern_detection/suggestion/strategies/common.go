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
