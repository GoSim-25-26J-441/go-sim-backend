package amg_apd

import "github.com/gin-gonic/gin"

func Register(r *gin.Engine) {
	v1 := r.Group("/api/v1/amg-apd")

	v1.POST("/analyze-raw", AnalyzeRaw)
	v1.POST("/analyze", AnalyzeUpload)

	v1.POST("/suggestions", SuggestionPreview)
	v1.POST("/apply-suggestions", SuggestionApply)
}
