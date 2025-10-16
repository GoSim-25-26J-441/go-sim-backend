package service

import (
	"context"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/graph/export"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/parser"
)

type Result struct {
	GraphJSON []byte
	Findings  []detection.Finding
}

func AnalyzeYAML(ctx context.Context, specYAML []byte) (Result, error) {
	_ = ctx
	spec, err := parser.FromYAML(specYAML)
	if err != nil {
		return Result{}, err
	}
	g := mapper.ToGraph(spec)
	j, err := export.ToJSON(g)
	if err != nil {
		return Result{}, err
	}
	ff := detection.RunAll(g, spec)
	return Result{GraphJSON: j, Findings: ff}, nil
}
