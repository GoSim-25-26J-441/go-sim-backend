package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	DB        string    `json:"db,omitempty"`
}

type HealthHandler struct {
	serviceName string
	version     string
	db          *pgxpool.Pool
}

func NewHealthHandler(serviceName, version string, db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		version:     version,
		db:          db,
	}
}

func (h *HealthHandler) HealthCheck(c *gin.Context) {
	resp := HealthResponse{
		Timestamp: time.Now().UTC(),
		Service:   h.serviceName,
		Version:   h.version,
	}

	// If no DB injected, keep health OK (useful for local/unit tests)
	if h.db == nil {
		resp.Status = "healthy"
		c.JSON(http.StatusOK, resp)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		resp.Status = "unhealthy"
		resp.DB = "down"
		c.JSON(http.StatusServiceUnavailable, resp)
		return
	}

	resp.Status = "healthy"
	resp.DB = "up"
	c.JSON(http.StatusOK, resp)
}

func (h *HealthHandler) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", h.HealthCheck)
	router.GET("/healthz", h.HealthCheck)
}
