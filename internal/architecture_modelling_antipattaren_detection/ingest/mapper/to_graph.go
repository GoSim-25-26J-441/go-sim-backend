package mapper

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"

func ToGraph(s domain.Spec) domain.Graph {
	nodes := make([]string, 0, len(s.Services))
	for name := range s.Services {
		nodes = append(nodes, name)
	}
	adj := make(map[string][]string, len(s.Services))
	for from, spec := range s.Services {
		adj[from] = append(adj[from], spec.Calls...)
	}
	return domain.Graph{Nodes: nodes, Adj: adj}
}
