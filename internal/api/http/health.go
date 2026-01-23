package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"database/sql"
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
	db          *sql.DB
}

func NewHealthHandler(serviceName, version string, db *sql.DB) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		version:     version,
		db:          db,
	}
}

func (h *HealthHandler) HealthCheck(c *gin.Context) {
	dbStatus := "disabled"
	if h.db != nil {
		pingCtx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		if err := h.db.PingContext(pingCtx); err != nil {
			dbStatus = "down"
		} else {
			dbStatus = "up"
		}
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Service:   h.serviceName,
		Version:   h.version,
		DB:        dbStatus,
	})
}

func (h *HealthHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/health", h.HealthCheck)
	r.GET("/healthz", h.HealthCheck)
}
