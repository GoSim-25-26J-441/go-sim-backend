package validator

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func Validate(s *parser.YSpec) error {
	if s == nil {
		return fmt.Errorf("spec is nil")
	}

	seen := map[string]bool{}
	for _, svc := range s.Services {
		n := strings.TrimSpace(svc.Name)
		if n == "" {
			return fmt.Errorf("service name is empty")
		}
		key := strings.ToLower(n)
		if seen[key] {
			return fmt.Errorf("duplicate service: %q", n)
		}
		seen[key] = true
	}

	for _, d := range s.Dependencies {
		if strings.TrimSpace(d.From) == "" || strings.TrimSpace(d.To) == "" {
			return fmt.Errorf("dependency has empty from/to")
		}
	}

	return nil
}
