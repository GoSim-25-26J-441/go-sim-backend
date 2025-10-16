package domain

type ServiceSpec struct {
	Calls       []string       `yaml:"calls"         json:"calls"`
	ChattyCalls map[string]int `yaml:"chatty_calls"  json:"chatty_calls,omitempty"`
	Writes      []string       `yaml:"writes"        json:"writes,omitempty"` // db names
	Reads       []string       `yaml:"reads"         json:"reads,omitempty"`  // db names
}

type Spec struct {
	Services map[string]ServiceSpec `yaml:"services" json:"services"`
}

type Graph struct {
	Nodes []string            `json:"nodes"`
	Adj   map[string][]string `json:"adj"`
}
