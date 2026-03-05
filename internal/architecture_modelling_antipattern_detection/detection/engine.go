package detection

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func RunAll(g *domain.Graph) ([]domain.Detection, error) {
	if g == nil {
		return nil, fmt.Errorf("detection: graph is nil")
	}

	var out []domain.Detection
	for _, det := range All() {
		ds, err := det.Detect(g)
		if err != nil {
			return nil, fmt.Errorf("detector %q failed: %w", det.Name(), err)
		}
		out = append(out, ds...)
	}
	return out, nil
}
