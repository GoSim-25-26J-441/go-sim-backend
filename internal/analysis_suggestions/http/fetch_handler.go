package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/fetchers"
	"github.com/gin-gonic/gin"
)

type FetchHandler struct{}

func NewFetchHandler() *FetchHandler {
	return &FetchHandler{}
}

func (h *FetchHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/fetch-prices", h.FetchAllPrices)
}

func (h *FetchHandler) FetchAllPrices(c *gin.Context) {
	start := time.Now()
	log.Println("Starting price fetcher from API...")

	go func() {
		if err := fetchers.RunAll(context.Background(), "out"); err != nil {
			log.Printf("Fetcher process failed: %v", err)
		} else {
			log.Println("Fetcher process completed successfully")
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":   "started",
		"message":  "Fetcher started successfully",
		"duration": time.Since(start).String(),
	})
}
