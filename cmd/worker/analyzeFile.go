package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/service"
)

// analyzeFile reads a YAML spec, runs the pipeline, and prints JSON to stdout.
func analyzeFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	res, err := service.AnalyzeYAML(context.Background(), b)
	if err != nil {
		return err
	}

	var graph any
	_ = json.Unmarshal(res.GraphJSON, &graph)

	out := struct {
		Graph    any   `json:"graph"`
		Findings any   `json:"findings"`
	}{
		Graph:    graph,
		Findings: res.Findings, // already JSON-encodable
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
