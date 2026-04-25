package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// validateScenarioPreflight calls simulation-core POST /v1/scenarios:validate (authoritative preflight).
// On success it returns the engine validation summary; local YAML parsing is not used as a gate.
func (h *Handler) validateScenarioPreflight(ctx context.Context, scenarioYAML string) (*ScenarioValidationResult, error) {
	if h.engineClient == nil {
		return nil, fmt.Errorf("%w: engine client not configured", ErrScenarioValidationUnavailable)
	}
	return h.engineClient.ValidateScenario(ctx, scenarioYAML)
}

// writeScenarioValidationError writes a JSON error for validateScenarioPreflight failures. Returns true if err was handled.
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
