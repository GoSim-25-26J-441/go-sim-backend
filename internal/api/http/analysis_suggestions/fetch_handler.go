package analysis_suggestions

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

type FetchHandler struct{}

func NewFetchHandler() *FetchHandler {
	return &FetchHandler{}
}

func (h *FetchHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/fetch-prices", h.FetchAllPrices)
	router.GET("/fetch-prices", h.FetchAllPrices)
}

func (h *FetchHandler) FetchAllPrices(c *gin.Context) {
	start := time.Now()

	fetcherPath := filepath.Join("internal", "analysis_suggestions", "fetchers", "fetcher_manager.go")

	log.Println("▶️ Starting price fetcher from API...")

	cmd := exec.Command("go", "run", fetcherPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("❌ Failed to start fetcher: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to start fetcher_manager.go",
			"error":   err.Error(),
		})
		return
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("❌ Fetcher process failed: %v", err)
		} else {
			log.Println("✅ Fetcher process completed successfully")
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":   "started",
		"message":  "Fetcher started successfully",
		"duration": time.Since(start).String(),
	})
}
