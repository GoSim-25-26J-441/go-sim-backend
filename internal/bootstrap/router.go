package bootstrap

import (
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	apiroutes "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/routes"

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

	apiroutes.RegisterV1(r, apiroutes.V1Deps{
		DB:          dep.DB,
		UpstreamURL: dep.UpstreamURL,
		OllamaURL:   dep.OllamaURL,
	})

	return r
}
