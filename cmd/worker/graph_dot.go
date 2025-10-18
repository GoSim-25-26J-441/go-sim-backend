package main

import (
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/graph/export"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/parser"
)

func writeDOT(inPath, outPath string) error {
	b, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	spec, err := parser.FromYAML(b)
	if err != nil {
		return err
	}
	g := mapper.ToGraph(spec)
	dot, err := export.ToDOT(g)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, dot, 0o644)
}
