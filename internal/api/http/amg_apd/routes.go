package amg_apd

import "github.com/gin-gonic/gin"

func Register(r *gin.Engine) {
	v1 := r.Group("/api/v1/amg-apd")

	// Existing
	v1.POST("/analyze-raw", AnalyzeRaw)
	v1.POST("/analyze", AnalyzeUpload)

	// NEW: Suggestions + Apply fixes (based on current YAML sent by frontend)
	v1.POST("/suggestions", SuggestionPreview)
	v1.POST("/apply-suggestions", SuggestionApply)
}
