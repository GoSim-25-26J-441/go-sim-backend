package detection

type Finding struct {
	Kind     string         `json:"kind"`
	Severity string         `json:"severity"`
	Summary  string         `json:"summary"`
	Nodes    []string       `json:"nodes,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}
