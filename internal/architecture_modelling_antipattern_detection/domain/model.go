package domain

type Attrs map[string]any

type Node struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Kind NodeKind `json:"kind"`
	// attributes e.g., owner=true, team, etc.
	Attrs Attrs `json:"attrs,omitempty"`
}

type Edge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind EdgeKind `json:"kind"`
	// weights/meta e.g., endpoints, rate_per_min, per_item=true
	Attrs Attrs `json:"attrs,omitempty"`
}

type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges []*Edge          `json:"edges"`
	// adjacency for algorithms
	Out map[string][]*Edge `json:"-"`
	In  map[string][]*Edge `json:"-"`
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: map[string]*Node{},
		Edges: []*Edge{},
		Out:   map[string][]*Edge{},
		In:    map[string][]*Edge{},
	}
}

func (g *Graph) AddNode(n *Node) {
	if _, ok := g.Nodes[n.ID]; !ok {
		g.Nodes[n.ID] = n
	}
}

func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
	g.Out[e.From] = append(g.Out[e.From], e)
	g.In[e.To] = append(g.In[e.To], e)
}

type Detection struct {
	Kind     AntiPatternKind `json:"kind"`
	Severity Severity        `json:"severity"`
	Title    string          `json:"title"`
	Summary  string          `json:"summary"`
	Nodes    []string        `json:"nodes"`
	Edges    []int           `json:"edges"` // indexes into g.Edges
	Evidence Attrs           `json:"evidence,omitempty"`
}
