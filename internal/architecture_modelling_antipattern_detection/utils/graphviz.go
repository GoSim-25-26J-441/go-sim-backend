package utils

import (
	"fmt"
	"os"
	"os/exec"
)

func WriteFile(path, data string) error {
	return os.WriteFile(path, []byte(data), 0644)
}

func DotTo(pathDOT, outPath, format, dotBin string) error {
	if format == "" {
		format = "svg"
	}
	if dotBin == "" {
		dotBin = "dot"
	}

	if _, err := exec.LookPath(dotBin); err != nil {
		return fmt.Errorf("graphviz: dot binary not found (%q): %w", dotBin, err)
	}

	cmd := exec.Command(dotBin, "-T"+format, pathDOT, "-o", outPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
