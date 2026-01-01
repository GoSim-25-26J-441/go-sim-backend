package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	_ "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection/rules"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/graph/export"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/utils"
)

type Result struct {
	Graph      *domain.Graph      `json:"graph" yaml:"graph"`
	DOTPath    string             `json:"dot_path" yaml:"dot_path"`
	SVGPath    string             `json:"svg_path" yaml:"svg_path"`
	Detections []domain.Detection `json:"detections" yaml:"detections"`
}

func AnalyzeYAML(path string, outDir string, title string, dotBin string) (*Result, error) {
	ys, err := parser.ParseYAML(path)
	if err != nil { return nil, err }
	g := mapper.ToGraph(ys)

	if outDir == "" { outDir = "out" }
	_ = os.MkdirAll(outDir, 0755)

	// export DOT/SVG
	dot := export.ToDOT(g, title)
	dotPath := filepath.Join(outDir, "graph.dot")
	if err := utils.WriteFile(dotPath, dot); err != nil { return nil, err }
	svgPath := filepath.Join(outDir, "graph.svg")
	if dotBin == "" { dotBin = "dot" } // safe default
	if err := utils.DotTo(dotPath, svgPath, "svg", dotBin); err != nil {
		return nil, fmt.Errorf("graphviz render: %w", err)
	}

	// detection
	var all []domain.Detection
	for _, d := range detection.All() {
		ds, err := d.Detect(g); if err != nil { return nil, err }
		all = append(all, ds...)
	}

	res := &Result{Graph: g, DOTPath: dotPath, SVGPath: svgPath, Detections: all}

	// NEW: persist full analysis in both JSON & YAML
	if err := export.WriteJSON(filepath.Join(outDir, "analysis.json"), res); err != nil { return nil, err }
	if err := export.WriteYAML(filepath.Join(outDir, "analysis.yaml"), res); err != nil { return nil, err }

	return res, nil
}
