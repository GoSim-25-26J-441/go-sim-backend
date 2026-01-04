package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/analyze-raw", h.AnalyzeRaw)
	rg.POST("/analyze", h.AnalyzeUpload)

	// Suggestions + Apply fixes (based on current YAML sent by frontend)
	rg.POST("/suggestions", h.SuggestionPreview)
	rg.POST("/apply-suggestions", h.SuggestionApply)
}

