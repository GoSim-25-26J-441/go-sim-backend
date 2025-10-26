package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection"
	_ "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection/rules"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/graph/export"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/utils"
)

type Result struct {
	Graph      *domain.Graph         `json:"graph"`
	DOTPath    string                `json:"dot_path"`
	SVGPath    string                `json:"svg_path"`
	Detections []domain.Detection    `json:"detections"`
}

func AnalyzeYAML(path string, outDir string, title string, dotBin string) (*Result, error) {
	ys, err := parser.ParseYAML(path)
	if err != nil { return nil, err }
	g := mapper.ToGraph(ys)

	// persist to Neo4j (optional) — can be plugged here later

	// export DOT/SVG
	if outDir == "" { outDir = "out" }
	_ = os.MkdirAll(outDir, 0755)
	dot := export.ToDOT(g, title)
	dotPath := filepath.Join(outDir, "graph.dot")
	if err := utils.WriteFile(dotPath, dot); err != nil { return nil, err }
	svgPath := filepath.Join(outDir, "graph.svg")
	if err := utils.DotTo(dotPath, svgPath, "svg", dotBin); err != nil {
		return nil, fmt.Errorf("graphviz render: %w", err)
	}

	// detection (over graph)
	var all []domain.Detection
	for _, d := range detection.All() {
		ds, err := d.Detect(g); if err != nil { return nil, err }
		all = append(all, ds...)
	}

	return &Result{Graph:g, DOTPath:dotPath, SVGPath:svgPath, Detections:all}, nil
}
