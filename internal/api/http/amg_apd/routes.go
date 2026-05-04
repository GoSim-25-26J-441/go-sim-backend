package amg_apd

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/amg_apd_version"
)

// Register mounts AMG-APD routes. Pass db for versioning/storage (uses Postgres from .env).
// versionRepo can be nil to create one from db; pass a repo when sharing with other handlers (e.g. project delete cascade).
func Register(r *gin.Engine, db *sql.DB, versionRepo *amg_apd_version.Repo) {
	if versionRepo == nil {
		versionRepo = amg_apd_version.NewRepo(db)
	}
	v1 := r.Group("/api/v1/amg-apd")
	h := NewHandlers(versionRepo)

	v1.POST("/analyze-raw", h.AnalyzeRaw)
	v1.POST("/analyze", h.AnalyzeUpload)
	v1.POST("/update-version-analysis", h.UpdateVersionAnalysis)

	v1.GET("/versions", h.ListVersions)
	v1.GET("/versions/compare", h.CompareVersions)
	v1.GET("/versions/:id", h.GetVersion)
	v1.PATCH("/versions/:id", h.PatchVersion)
	v1.DELETE("/versions/:id", h.DeleteVersion)
	v1.GET("/projects/:project_public_id/latest", h.GetLatestForProject)

	v1.POST("/suggestions", SuggestionPreview)
	v1.POST("/apply-suggestions", SuggestionApply)
}
