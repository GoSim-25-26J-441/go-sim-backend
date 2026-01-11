package parser

import (
	"os"

	"gopkg.in/yaml.v3"
)

type YSpec struct {
	
	Note       string                 `yaml:"__note,omitempty"`
	APIs       []YAPI                 `yaml:"apis,omitempty"`
	Configs    map[string]any         `yaml:"configs,omitempty"`
	Conflicts  []any                  `yaml:"conflicts,omitempty"`
	Constraints map[string]any        `yaml:"constraints,omitempty"`
	Datastores []YDatastore           `yaml:"datastores,omitempty"`
	Dependencies []YDependency        `yaml:"dependencies,omitempty"`
	DeploymentHints map[string]any    `yaml:"deploymentHints,omitempty"`
	Gaps       []any                  `yaml:"gaps,omitempty"`
	Metadata   map[string]any         `yaml:"metadata,omitempty"`
	Topics     []YTopic               `yaml:"topics,omitempty"`
	Trace      []any                  `yaml:"trace,omitempty"`


	Services []YService `yaml:"services"`

	Databases []YDatabase `yaml:"databases,omitempty"`
}


type YAPI struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol"`
}

type YDatastore struct {
	Name string `yaml:"name"`
	Type string `yaml:"type,omitempty"`
}

type YDependency struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	Kind string `yaml:"kind,omitempty"`
	Sync bool   `yaml:"sync,omitempty"`
}

type YTopic struct {
	Name string `yaml:"name"`
}


type YService struct {
	Name      string     `yaml:"name"`
	Type      string     `yaml:"type,omitempty"`
	Calls     []YCall    `yaml:"calls,omitempty"`
	Databases YDatabases `yaml:"databases,omitempty"`
}

type YDatabase struct {
	Name string `yaml:"name"`
}

type YDatabases struct {
	Reads  []string `yaml:"reads,omitempty"`
	Writes []string `yaml:"writes,omitempty"`
}

type YCall struct {
	To         string   `yaml:"to"`
	Endpoints  []string `yaml:"endpoints,omitempty"`
	RatePerMin int      `yaml:"rate_per_min,omitempty"`
	PerItem    bool     `yaml:"per_item,omitempty"`
}

func ParseYAML(path string) (*YSpec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseYAMLBytes(b)
}

func ParseYAMLBytes(b []byte) (*YSpec, error) {
	var s YSpec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func ParseYAMLString(s string) (*YSpec, error) {
	return ParseYAMLBytes([]byte(s))
}
