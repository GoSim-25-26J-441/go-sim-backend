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

func getIncomingDir() string {
	if d := os.Getenv("AMG_APD_INCOMING_DIR"); d != "" {
		return d
	}
	return "/app/incoming"
}

func getOutDir() string {
	if d := os.Getenv("AMG_APD_OUT_DIR"); d != "" {
		return d
	}
	return "/app/out"
}

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
		req.OutDir = getOutDir()
	}
	if req.Title == "" {
		req.Title = "Uploaded"
	}

	incoming := getIncomingDir()
	_ = os.MkdirAll(incoming, 0o755)
	tmp := filepath.Join(incoming, utils.NewID()+".yaml")
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

	outDir := c.DefaultPostForm("out_dir", getOutDir())

	title := c.PostForm("title")
	if title == "" {
		base := filepath.Base(file.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
		if title == "" {
			title = "Uploaded"
		}
	}

	incoming := getIncomingDir()
	_ = os.MkdirAll(incoming, 0o755)
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".yaml"
	}
	tmpPath := filepath.Join(incoming, utils.NewID()+ext)
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
