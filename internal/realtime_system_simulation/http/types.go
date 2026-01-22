package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for simulation runs
type Handler struct {
	simService          *service.SimulationService
	simulationEngineURL string
	engineClient        *SimulationEngineClient
	callbackSecret      string // Secret for authenticating callbacks from simulation engine
}

// New creates a new Handler
func New(simService *service.SimulationService, simulationEngineURL string, callbackSecret string) *Handler {
	return &Handler{
		simService:          simService,
		simulationEngineURL: simulationEngineURL,
		engineClient:        NewSimulationEngineClient(simulationEngineURL),
		callbackSecret:      callbackSecret,
	}
}

// RegisterEngineCallbackRoutes registers callback routes (called by simulation engine, not end users)
func (h *Handler) RegisterEngineCallbackRoutes(rg *gin.RouterGroup) {
	rg.POST("/runs/callback", h.EngineRunCallback)
	rg.POST("/runs/:id/callback", h.EngineRunCallbackByID)
}
