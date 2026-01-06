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
		RunAnalyze(os.Args[2:])
	default:
		log.Fatal("unknown command")
	}
}
