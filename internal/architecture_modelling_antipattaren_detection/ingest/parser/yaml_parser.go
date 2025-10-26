package parser

import (
	"os"

	"gopkg.in/yaml.v3"
)

type YSpec struct {
	Services  []YService  `yaml:"services"`
	Databases []YDatabase `yaml:"databases"`
}

type YService struct {
	Name      string      `yaml:"name"`
	Calls     []YCall     `yaml:"calls"`
	Databases YDatabases  `yaml:"databases"`
}

type YDatabase struct {
	Name string `yaml:"name"`
}

type YDatabases struct {
	Reads  []string `yaml:"reads"`
	Writes []string `yaml:"writes"`
}

type YCall struct {
	To          string   `yaml:"to"`
	Endpoints   []string `yaml:"endpoints"`
	RatePerMin  int      `yaml:"rate_per_min"`
	PerItem     bool     `yaml:"per_item"`
}

func ParseYAML(path string) (*YSpec, error) {
	b, err := os.ReadFile(path)
	if err != nil { return nil, err }
	var s YSpec
	if err := yaml.Unmarshal(b, &s); err != nil { return nil, err }
	return &s, nil
}
