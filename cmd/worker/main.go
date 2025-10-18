package main

import (
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Println("usage: worker analyze <yaml> | worker dot <yaml> <out.dot>")
		return
	}

	switch os.Args[1] {
	case "analyze":
		if len(os.Args) < 3 {
			log.Fatal("usage: worker analyze <yaml-file>")
		}
		if err := analyzeFile(os.Args[2]); err != nil {
			log.Fatal(err)
		}
	case "dot":
		if len(os.Args) < 4 {
			log.Fatal("usage: worker dot <yaml-file> <out.dot>")
		}
		if err := writeDOT(os.Args[2], os.Args[3]); err != nil {
			log.Fatal(err)
		}
	default:
		log.Println("unknown command")
	}
}
