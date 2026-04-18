package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	simconfig "github.com/GoSim-25-26J-441/simulation-core/pkg/config"
	"github.com/gin-gonic/gin"
)

type parseScenarioYAMLError struct {
	err error
}

func (e *parseScenarioYAMLError) Error() string { return e.err.Error() }
func (e *parseScenarioYAMLError) Unwrap() error { return e.err }

// validateScenarioPreflight runs local ParseScenarioYAML then simulation-core POST /v1/scenarios:validate.
func (h *Handler) validateScenarioPreflight(ctx context.Context, scenarioYAML string) error {
	if _, err := simconfig.ParseScenarioYAML([]byte(scenarioYAML)); err != nil {
		return &parseScenarioYAMLError{err: err}
	}
	if h.engineClient == nil {
		return fmt.Errorf("%w: engine client not configured", ErrScenarioValidationUnavailable)
	}
	_, err := h.engineClient.ValidateScenario(ctx, scenarioYAML)
	return err
}

// writeScenarioValidationError writes a JSON error for validateScenarioPreflight failures. Returns true if err was handled.
func (h *Handler) writeScenarioValidationError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var parseErr *parseScenarioYAMLError
	if errors.As(err, &parseErr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario_yaml", "details": parseErr.err.Error()})
		return true
	}
	var sve *ScenarioValidationError
	if errors.As(err, &sve) && sve.Result != nil {
		details := ""
		if len(sve.Result.Errors) > 0 {
			details = sve.Result.Errors[0].Message
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":      "invalid scenario_yaml",
			"details":    details,
			"validation": sve.Result,
		})
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
