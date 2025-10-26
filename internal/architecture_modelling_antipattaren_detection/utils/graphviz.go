package utils

import (
	"os"
	"os/exec"
)

func WriteFile(path, data string) error {
	return os.WriteFile(path, []byte(data), 0644)
}

func DotTo(pathDOT, outPath, format, dotBin string) error {
	if format == "" { format = "svg" }
	cmd := exec.Command(dotBin, "-T"+format, pathDOT, "-o", outPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
