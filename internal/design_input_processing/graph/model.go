package graph

// Graph is the “canonical graph” used for AMG/APD, Neo4j, GraphViz.
type Graph struct {
	Nodes []Node
	Edges []Edge
}

type Node struct {
	ID   string
	Name string
	Kind string
}

type Edge struct {
	From     string
	To       string
	Protocol string
	RPS      float64
}
