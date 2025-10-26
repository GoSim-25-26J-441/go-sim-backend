package main

import (
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: worker analyze <yamlPath> [outDir]")
	}
	switch os.Args[1] {
	case "analyze":
		// RunAnalyze is defined in amg_apd_analyze.go in the same package.
		RunAnalyze(os.Args[2:])
	default:
		log.Fatal("unknown command")
	}
}
