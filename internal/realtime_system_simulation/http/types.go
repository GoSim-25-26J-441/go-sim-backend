package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// Handler handles HTTP requests for simulation runs
type Handler struct {
	simService          *service.SimulationService
	simulationEngineURL string
	engineClient        *SimulationEngineClient
}

// New creates a new Handler
func New(simService *service.SimulationService, simulationEngineURL string) *Handler {
	return &Handler{
		simService:          simService,
		simulationEngineURL: simulationEngineURL,
		engineClient:        NewSimulationEngineClient(simulationEngineURL),
	}
}
