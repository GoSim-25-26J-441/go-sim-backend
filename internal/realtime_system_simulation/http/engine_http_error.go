package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// EngineHTTPError is returned when the simulation engine HTTP API responds with a non-success status.
type EngineHTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *EngineHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("simulation engine returned status %d: %s", e.StatusCode, string(e.Body))
}

// HTTPStatusForEngineCreate maps simulation-core create-run responses to backend HTTP status codes.
// 400/401/404 pass through; 429 and 409 pass through; other errors become 502.
func HTTPStatusForEngineCreate(engineStatus int) int {
	switch engineStatus {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound,
		http.StatusTooManyRequests, http.StatusConflict:
		return engineStatus
	default:
		return http.StatusBadGateway
	}
}

// HTTPStatusForEnginePOST maps generic engine POST responses (e.g. renew-lease).
func HTTPStatusForEnginePOST(engineStatus int) int {
	switch engineStatus {
	case http.StatusOK, http.StatusNoContent:
		return engineStatus
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound,
		http.StatusTooManyRequests, http.StatusConflict, http.StatusPreconditionFailed:
		return engineStatus
	default:
		return http.StatusBadGateway
	}
}

// ExtractEngineErrorMessage parses a JSON error or message field from an engine error body; otherwise returns trimmed raw text.
func ExtractEngineErrorMessage(body []byte) string {
	var m struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil {
		if m.Error != "" {
			return m.Error
		}
		if m.Message != "" {
			return m.Message
		}
	}
	return strings.TrimSpace(string(body))
}
