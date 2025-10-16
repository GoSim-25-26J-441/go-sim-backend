package rules

type Finding struct {
	Kind     string
	Severity string
	Summary  string
	Nodes    []string
	Meta     map[string]any
}
