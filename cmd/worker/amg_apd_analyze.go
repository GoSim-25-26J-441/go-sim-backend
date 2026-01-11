package main

import (
	"fmt"
	"log"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
)

func RunAnalyze(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: worker analyze <yamlPath> [outDir] [title]")
	}

	yamlPath := args[0]

	outDir := "out"
	if len(args) > 1 && args[1] != "" {
		outDir = args[1]
	}

	title := "Architecture"
	if len(args) > 2 && args[2] != "" {
		title = args[2]
	}

	res, err := service.AnalyzeYAML(yamlPath, outDir, title, os.Getenv("DOT_BIN"))
	if err != nil {
		log.Fatalf("analyze failed: %v", err)
	}

	fmt.Printf("Wrote: %s, %s\n", res.DOTPath, res.SVGPath)
	fmt.Printf("Detections (%d):\n", len(res.Detections))
	for _, d := range res.Detections {
		fmt.Printf(" - [%s] %s: %s\n", d.Kind, d.Title, d.Summary)
	}
}
