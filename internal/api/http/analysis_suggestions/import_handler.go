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

type ImportHandler struct{}

func NewImportHandler() *ImportHandler { return &ImportHandler{} }

func (h *ImportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/import-prices", h.RunImporter)
}

func (h *ImportHandler) RunImporter(c *gin.Context) {
	start := time.Now()

	wd, err := os.Getwd()
	if err != nil {
		log.Printf("❌ failed to get working dir: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "failed to get working directory", "error": err.Error()})
		return
	}

	importerPath := filepath.Join(wd, "internal", "analysis_suggestions", "importer", "import_prices.go")
	log.Printf("▶️ Starting importer: %s", importerPath)

	cmd := exec.Command("go", "run", importerPath, "--dir", "out")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("❌ failed to start importer: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "failed to start importer", "error": err.Error()})
		return
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("❌ importer process failed: %v", err)
		} else {
			log.Printf("✅ importer process finished successfully")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":   "started",
		"message":  "importer started",
		"duration": time.Since(start).String(),
	})
}
