package graph

import "fmt"

type ServiceSpec struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type DependencySpec struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Protocol string  `json:"protocol"`
	RPS      float64 `json:"rps"`
}

// Architecture is your canonical spec shape.
// Adjust field names to match what uigp-service returns.
type Architecture struct {
	Services     []ServiceSpec    `json:"services"`
	Dependencies []DependencySpec `json:"dependencies"`
}

func FromSpec(spec Architecture) (*Graph, error) {
	g := &Graph{}

	idSet := map[string]bool{}
	for _, s := range spec.Services {
		if s.ID == "" {
			return nil, fmt.Errorf("service with empty id")
		}
		if idSet[s.ID] {
			return nil, fmt.Errorf("duplicate service id %q", s.ID)
		}
		idSet[s.ID] = true

		kind := s.Kind
		if kind == "" {
			kind = "service"
		}

		g.Nodes = append(g.Nodes, Node{
			ID:   s.ID,
			Name: s.Name,
			Kind: kind,
		})
	}

	for _, d := range spec.Dependencies {
		if d.From == "" || d.To == "" {
			continue
		}
		proto := d.Protocol
		if proto == "" {
			proto = "REST"
		}
		g.Edges = append(g.Edges, Edge{
			From:     d.From,
			To:       d.To,
			Protocol: proto,
			RPS:      d.RPS,
		})
	}

	return g, nil
}
