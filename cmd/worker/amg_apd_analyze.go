package main

import (
	"fmt"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/service"
)

func RunAnalyze(args []string) {
	if len(args) < 1 {
		panic("usage: analyze <yamlPath> [outDir]")
	}
	yaml := args[0]
	out := "out"
	if len(args) > 1 {
		out = args[1]
	}
	res, err := service.AnalyzeYAML(yaml, out, "Architecture", os.Getenv("DOT_BIN"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Wrote: %s, %s\n", res.DOTPath, res.SVGPath)
	fmt.Printf("Detections (%d):\n", len(res.Detections))
	for _, d := range res.Detections {
		fmt.Printf(" - [%s] %s: %s\n", d.Kind, d.Title, d.Summary)
	}
}
