package detection

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"

type Detector interface {
	Name() string
	Detect(g *domain.Graph) ([]domain.Detection, error)
}
