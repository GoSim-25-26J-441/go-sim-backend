package routes

import (
	dipdiagrams "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/diagrams"
	mw "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/middleware"

	"github.com/gin-gonic/gin"
)

type V1Deps struct {
	UpstreamURL string
	OllamaURL   string
}

func RegisterV1(api *gin.RouterGroup, dep V1Deps) {
	_ = dep

	dip := api.Group("/design-input")
	dip.Use(mw.APIKeyMiddleware())
	dip.Use(mw.RequestIDMiddleware())

	dip.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true, "status": "design-input routes are disabled for now"})
	})
}

func RegisterProjectDiagramRoutes(projectsGroup *gin.RouterGroup, repo *dipdiagrams.Repo) {
	dipdiagrams.RegisterProjectDiagramRoutes(projectsGroup, repo)
}
