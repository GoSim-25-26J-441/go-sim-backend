package amg_apd

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	defaultOutDirOnce sync.Once
	defaultOutDirVal  string
)

// DefaultOutDir returns the default output directory for analysis (next to the executable).
// On EC2 with binary at /opt/go-sim-backend/api this returns /opt/go-sim-backend/out.
// If the executable path cannot be determined, returns "out" (cwd-relative).
func DefaultOutDir() string {
	defaultOutDirOnce.Do(func() {
		execPath, err := executablePath()
		if err != nil {
			defaultOutDirVal = "out"
			return
		}
		defaultOutDirVal = filepath.Join(filepath.Dir(execPath), "out")
	})
	return defaultOutDirVal
}

func executablePath() (string, error) {
	return os.Executable()
}
