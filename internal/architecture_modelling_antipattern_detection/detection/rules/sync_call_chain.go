package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type syncChain struct{}

func (s syncChain) Name() string { return "sync_call_chain" }

func (s syncChain) Detect(g *domain.Graph) ([]domain.Detection, error) {
	minEdges := 4
	if v := os.Getenv("DETECT_SYNC_CHAIN_MIN_EDGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minEdges = n
		}
	}

	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n != nil && n.Kind == domain.NodeService
	}

	nextSync := func(from string) []string {
		var out []string
		for _, e := range g.Out[from] {
			if e == nil || e.Kind != domain.EdgeCalls {
				continue
			}
			if !edgeIsSync(e) {
				continue
			}
			if isSvc(e.To) {
				out = append(out, e.To)
			}
		}
		return out
	}

	best := []string{}

	var dfs func(cur string, visited map[string]bool, path []string)
	dfs = func(cur string, visited map[string]bool, path []string) {
		if len(path) > len(best) {
			best = append([]string{}, path...)
		}
		for _, nxt := range nextSync(cur) {
			if visited[nxt] {
				continue
			}
			visited[nxt] = true
			dfs(nxt, visited, append(path, nxt))
			delete(visited, nxt)
		}
	}

	for id, n := range g.Nodes {
		if n == nil || n.Kind != domain.NodeService {
			continue
		}
		visited := map[string]bool{id: true}
		dfs(id, visited, []string{id})
	}

	edges := len(best) - 1
	if edges >= minEdges {
		return []domain.Detection{{
			Kind:     domain.APSyncCallChain,
			Severity: domain.SeverityMedium,
			Title:    "Sync call chain",
			Summary:  "Long synchronous dependency chain can amplify latency/failure impact",
			Nodes:    best,
			Evidence: domain.Attrs{"edges": edges, "min_edges": minEdges},
		}}, nil
	}

	return nil, nil
}

func init() { detection.Register(syncChain{}) }
