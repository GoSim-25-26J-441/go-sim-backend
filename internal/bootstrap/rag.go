package bootstrap

import diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"

func LoadRAG(snippetsDir string) error {
	return diprag.Load(snippetsDir)
}
