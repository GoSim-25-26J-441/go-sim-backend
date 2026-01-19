package bootstrap

import (
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	diphttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http"
	middleware "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RouterDeps struct {
	ServiceName string
	Version     string
	UpstreamURL string
	OllamaURL   string
	DB          *pgxpool.Pool
}

func BuildRouter(dep RouterDeps) *gin.Engine {
	r := gin.Default()

	healthHandler := httpapi.NewHealthHandler(dep.ServiceName, dep.Version, dep.DB)
	healthHandler.RegisterRoutes(r)

	api := r.Group("/api/v1")

	dip := api.Group("/design-input")
	dip.Use(middleware.APIKeyMiddleware())
	dip.Use(middleware.RequestIDMiddleware())

	dipHandler := diphttp.New(dep.UpstreamURL, dep.OllamaURL)
	dipHandler.Register(dip)

	return r
}
