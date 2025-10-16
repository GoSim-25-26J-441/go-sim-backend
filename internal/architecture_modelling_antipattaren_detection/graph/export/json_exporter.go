package export

import (
	"encoding/json"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

func ToJSON(g domain.Graph) ([]byte, error) {
	return json.Marshal(g)
}
