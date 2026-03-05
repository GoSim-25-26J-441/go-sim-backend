package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	"github.com/gin-gonic/gin"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := httpapi.NewHealthHandler("test-service", "1.0.0")
	handler.RegisterRoutes(router)

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	var response httpapi.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %s", response.Status)
	}

	if response.Service != "test-service" {
		t.Errorf("expected service 'test-service', got %s", response.Service)
	}

	if response.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", response.Version)
	}
}

func TestHealthCheckMethodNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.HandleMethodNotAllowed = true

	handler := httpapi.NewHealthHandler("test-service", "1.0.0")
	handler.RegisterRoutes(router)

	req, err := http.NewRequest("POST", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusMethodNotAllowed)
	}
}