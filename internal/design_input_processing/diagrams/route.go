package diagrams

import "github.com/gin-gonic/gin"

func RegisterProjectRoutes(projectsGroup *gin.RouterGroup, repo *Repo) {
	h := &handler{repo: repo}

	projectsGroup.POST("/:public_id/diagram", h.createVersion)
	projectsGroup.GET("/:public_id/diagram/latest", h.latest)
}
