package rules

import (
	"os"
	"strconv"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type sharedDB struct{}

func (s sharedDB) Name() string { return "shared_database" }

func isDBName(id string) bool {
	x := strings.ToLower(strings.TrimSpace(id))
	return x == "database" || strings.Contains(x, "database") || strings.HasSuffix(x, "-db") || strings.Contains(x, " db")
}

func (s sharedDB) Detect(g *domain.Graph) ([]domain.Detection, error) {
	minClients := 2
	if v := os.Getenv("DETECT_SHARED_DB_MIN_CLIENTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minClients = n
		}
	}

	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n.Kind == domain.NodeService
	}

	var out []domain.Detection

	for id, n := range g.Nodes {
		if n.Kind != domain.NodeService {
			continue
		}
		if !isDBName(id) {
			continue
		}

		clientsSet := map[string]bool{}
		for _, e := range g.In[id] {
			if e.Kind != domain.EdgeCalls {
				continue
			}
			if isSvc(e.From) && e.From != id {
				clientsSet[e.From] = true
			}
		}

		clients := make([]string, 0, len(clientsSet))
		for c := range clientsSet {
			clients = append(clients, c)
		}

		if len(clients) >= minClients {
			nodes := append([]string{id}, clients...)
			sev := domain.SeverityMedium
			if len(clients) >= 3 {
				sev = domain.SeverityHigh
			}
			out = append(out, domain.Detection{
				Kind:     "shared_database",
				Severity: sev,
				Title:    "Shared database",
				Summary:  "Multiple services depend on the same database node",
				Nodes:    nodes,
				Evidence: domain.Attrs{"db": id, "clients": len(clients), "min_clients": minClients},
			})
		}
	}

	return out, nil
}

func init() { detection.Register(sharedDB{}) }
