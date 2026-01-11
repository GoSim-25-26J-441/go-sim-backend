package amg_apd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/utils"
)

type analyzeRawReq struct {
	YAML   string `json:"yaml"`
	Title  string `json:"title"`
	OutDir string `json:"out_dir"`
}

func AnalyzeRaw(c *gin.Context) {
	var req analyzeRawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid json body")
		return
	}
	if req.YAML == "" {
		c.String(http.StatusBadRequest, "yaml is required")
		return
	}
	if req.OutDir == "" {
		req.OutDir = "/app/out"
	}
	if req.Title == "" {
		req.Title = "Uploaded"
	}

	_ = os.MkdirAll("/app/incoming", 0o755)
	tmp := filepath.Join("/app/incoming", utils.NewID()+".yaml")
	if err := os.WriteFile(tmp, []byte(req.YAML), 0o644); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("write tmp failed: %v", err))
		return
	}
	defer os.Remove(tmp)

	res, err := service.AnalyzeYAML(tmp, req.OutDir, req.Title, os.Getenv("DOT_BIN"))
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", err))
		return
	}

	c.JSON(http.StatusOK, res)
}

func AnalyzeUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}

	outDir := c.DefaultPostForm("out_dir", "/app/out")

	title := c.PostForm("title")
	if title == "" {
		base := filepath.Base(file.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
		if title == "" {
			title = "Uploaded"
		}
	}

	_ = os.MkdirAll("/app/incoming", 0o755)
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".yaml"
	}
	tmpPath := filepath.Join("/app/incoming", utils.NewID()+ext)
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("saving uploaded file failed: %v", err))
		return
	}
	defer os.Remove(tmpPath)

	res, err := service.AnalyzeYAML(tmpPath, outDir, title, os.Getenv("DOT_BIN"))
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", err))
		return
	}

	c.JSON(http.StatusOK, res)
}
