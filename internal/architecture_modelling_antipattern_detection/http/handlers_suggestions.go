package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
)

func (h *Handler) SuggestionPreview(c *gin.Context) {
	var req SuggestionPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid json body")
		return
	}
	if req.YAML == "" {
		c.String(http.StatusBadRequest, "yaml is required")
		return
	}
	if req.OutDir == "" {
		req.OutDir = "/app/out"
	}
	if req.Title == "" {
		req.Title = "Architecture"
	}

	res, err := service.PreviewSuggestionsYAMLString(req.YAML, req.OutDir, req.Title)
	if err != nil {
		c.String(http.StatusBadRequest, "suggestion preview failed: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) SuggestionApply(c *gin.Context) {
	var req SuggestionApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid json body")
		return
	}
	if req.YAML == "" {
		c.String(http.StatusBadRequest, "yaml is required")
		return
	}
	if req.OutDir == "" {
		req.OutDir = "/app/out"
	}
	if req.Title == "" {
		req.Title = "Architecture"
	}
	if req.JobID == "" {
		req.JobID = "adhoc"
	}

	res, err := service.ApplySuggestionsYAMLString(req.JobID, req.YAML, req.OutDir, req.Title)
	if err != nil {
		c.String(http.StatusBadRequest, "apply suggestions failed: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}

