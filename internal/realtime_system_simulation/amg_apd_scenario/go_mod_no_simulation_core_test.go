package amg_apd_scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoModHasNoSimulationCoreDependency(t *testing.T) {
	root := findGoModDir(t)
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "github.com/GoSim-25-26J-441/simulation-core") {
		t.Fatal("go.mod must not require or replace github.com/GoSim-25-26J-441/simulation-core; use HTTP validation against the simulation engine instead")
	}
}

func findGoModDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		st, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test working directory")
		}
		dir = parent
	}
}
