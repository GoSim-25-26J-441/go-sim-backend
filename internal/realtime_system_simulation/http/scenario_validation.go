package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// validateScenarioPreflight runs full simulation-core validation including placement and resource feasibility.
func (h *Handler) validateScenarioPreflight(ctx context.Context, scenarioYAML string) (*ScenarioValidationResult, error) {
	if h.engineClient == nil {
		return nil, fmt.Errorf("%w: engine client not configured", ErrScenarioValidationUnavailable)
	}
	return h.engineClient.ValidateScenario(ctx, scenarioYAML, ScenarioValidateModePreflight)
}

// validateScenarioDraft checks structure and references only (no placement/resource preflight).
func (h *Handler) validateScenarioDraft(ctx context.Context, scenarioYAML string) (*ScenarioValidationResult, error) {
	if h.engineClient == nil {
		return nil, fmt.Errorf("%w: engine client not configured", ErrScenarioValidationUnavailable)
	}
	return h.engineClient.ValidateScenario(ctx, scenarioYAML, ScenarioValidateModeDraft)
}

// ParseScenarioValidationEditorMode normalizes mode for PUT scenario / POST validate.
// defaultMode is used when mode is empty. Returns an error if mode is non-empty and not draft or preflight.
func ParseScenarioValidationEditorMode(mode string, defaultMode string) (string, error) {
	m := strings.TrimSpace(mode)
	if m == "" {
		return defaultMode, nil
	}
	if m == ScenarioValidateModeDraft || m == ScenarioValidateModePreflight {
		return m, nil
	}
	return "", fmt.Errorf(`validation mode must be "draft" or "preflight"`)
}

// writeScenarioValidationError writes a JSON error for engine scenario validation failures. Returns true if err was handled.
// Optional draftScenarioYAML is included on 422 (engine reported invalid scenario) for debugging generated drafts only.
func (h *Handler) writeScenarioValidationError(c *gin.Context, err error, draftScenarioYAML ...string) bool {
	if err == nil {
		return false
	}
	var sve *ScenarioValidationError
	if errors.As(err, &sve) && sve.Result != nil {
		details := ""
		if len(sve.Result.Errors) > 0 {
			details = sve.Result.Errors[0].Message
		}
		body := gin.H{
			"error":      "invalid scenario_yaml",
			"details":    details,
			"validation": sve.Result,
		}
		if len(draftScenarioYAML) > 0 && draftScenarioYAML[0] != "" {
			body["draft_scenario_yaml"] = draftScenarioYAML[0]
		}
		c.JSON(http.StatusUnprocessableEntity, body)
		return true
	}
	var eng *EngineHTTPError
	if errors.As(err, &eng) {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "scenario validation failed",
			"details": ExtractEngineErrorMessage(eng.Body),
		})
		return true
	}
	if errors.Is(err, ErrScenarioValidationUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "scenario validation unavailable",
			"details": err.Error(),
		})
		return true
	}
	return false
}
