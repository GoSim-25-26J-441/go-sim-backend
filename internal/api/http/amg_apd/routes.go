package amg_apd

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/amg_apd_version"
)

// Register mounts AMG-APD routes. Pass db for versioning/storage (uses Postgres from .env).
// user_id and chat_id are read from headers X-User-Id, X-Chat-Id (default TestUser123, TestChat123).
func Register(r *gin.Engine, db *sql.DB) {
	v1 := r.Group("/api/v1/amg-apd")
	versionRepo := amg_apd_version.NewRepo(db)
	h := NewHandlers(versionRepo)

	v1.POST("/analyze-raw", h.AnalyzeRaw)
	v1.POST("/analyze", h.AnalyzeUpload)

	v1.GET("/versions", h.ListVersions)
	v1.GET("/versions/compare", h.CompareVersions)
	v1.GET("/versions/:id", h.GetVersion)
	v1.DELETE("/versions/:id", h.DeleteVersion)
	v1.GET("/projects/:project_public_id/latest", h.GetLatestForProject)

	v1.POST("/suggestions", SuggestionPreview)
	v1.POST("/apply-suggestions", SuggestionApply)
}
