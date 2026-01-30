package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/service"
)

type Handler struct {
	upstreamClient *service.UpstreamClient
	jobService     *service.JobService
	graphService   *service.GraphService
	signalService  *service.SignalService
	ollamaURL      string
	upstreamURL    string
}

func New(upstreamURL, ollamaURL string) *Handler {
	upstreamClient := service.NewUpstreamClient(upstreamURL)
	return &Handler{
		upstreamClient: upstreamClient,
		jobService:     service.NewJobService(upstreamClient),
		graphService:   service.NewGraphService(upstreamClient),
		signalService:  service.NewSignalService(),
		ollamaURL:      ollamaURL,
		upstreamURL:    upstreamURL,
	}
}
