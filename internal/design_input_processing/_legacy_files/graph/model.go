package graph

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
