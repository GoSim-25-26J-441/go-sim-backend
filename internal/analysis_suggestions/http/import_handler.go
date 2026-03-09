package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/importer"
	"github.com/gin-gonic/gin"
)

type ImportHandler struct{}

func NewImportHandler() *ImportHandler { return &ImportHandler{} }

func (h *ImportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/import-prices", h.RunImporter)
}

func (h *ImportHandler) RunImporter(c *gin.Context) {
	start := time.Now()
	log.Printf("Starting importer (dir=out)")

	go func() {
		if err := importer.Run(context.Background(), "out", importer.DefaultBatchSize); err != nil {
			log.Printf("importer process failed: %v", err)
		} else {
			log.Printf("importer process finished successfully")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":   "started",
		"message":  "importer started",
		"duration": time.Since(start).String(),
	})
}
