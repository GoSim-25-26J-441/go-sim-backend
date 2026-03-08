package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
)

type SuggestRequest struct {
	UserID     string                `json:"user_id,omitempty"`
	ProjectID  string                `json:"project_id"`
	RunID      string                `json:"run_id"`
	Design     rules.DesignInput     `json:"design"`
	Simulation rules.SimulationInput `json:"simulation"`
	Candidates []rules.Candidate     `json:"candidates"`
	RuleFile   string                `json:"rule_file,omitempty"`
}

type SuggestResponse struct {
	Best      rules.CandidateScore   `json:"best"`
	AllScores []rules.CandidateScore `json:"all_scores"`
	StorageID string                 `json:"storage_id"`
}

type SuggestHandler struct {
	rulePath string
	db       *sql.DB
}

func NewSuggestHandler(rulePath string, db *sql.DB) *SuggestHandler {
	return &SuggestHandler{rulePath: rulePath, db: db}
}

func (h *SuggestHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/suggest", h.HandleSuggest)
}

func isZeroDesign(d rules.DesignInput) bool {
	return d.PreferredVCPU == 0 &&
		d.PreferredMemoryGB == 0 &&
		d.Workload.ConcurrentUsers == 0 &&
		d.Budget == 0
}

func hasDesignForProject(ctx context.Context, db *sql.DB, userID, projectID string) (bool, error) {
	if userID == "" || projectID == "" {
		return false, nil
	}
	const query = `
SELECT 1
FROM request_responses
WHERE user_id = $1
  AND project_id IS NOT DISTINCT FROM $2
  AND run_id IS NULL
LIMIT 1;
`
	var one int
	err := db.QueryRowContext(ctx, query, userID, projectID).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func loadLatestDesign(ctx context.Context, db *sql.DB, userID, projectID string) (rules.DesignInput, error) {
	var out rules.DesignInput

	if userID == "" || projectID == "" {
		return out, nil
	}

	const query = `
SELECT request
FROM request_responses
WHERE user_id = $1
  AND project_id IS NOT DISTINCT FROM $2
  AND run_id IS NULL
ORDER BY created_at DESC
LIMIT 1;
`

	var reqJSON []byte
	if err := db.QueryRowContext(ctx, query, userID, projectID).Scan(&reqJSON); err != nil {
		return out, err
	}

	var stored struct {
		Design rules.DesignInput `json:"design"`
	}
	if err := json.Unmarshal(reqJSON, &stored); err != nil {
		return out, err
	}

	return stored.Design, nil
}

func ensureDesignForProject(ctx context.Context, db *sql.DB, userID, projectID string, design rules.DesignInput) error {
	reqEnvelope := map[string]any{
		"design":     design,
		"simulation": map[string]any{"nodes": 0},
		"candidates": []any{},
	}
	reqJSON, err := json.Marshal(reqEnvelope)
	if err != nil {
		return err
	}
	projectIDVal := interface{}(projectID)
	if projectID == "" {
		projectIDVal = nil
	}
	const insertSQL = `
INSERT INTO request_responses (user_id, project_id, run_id, request, response, best_candidate, created_at)
VALUES ($1, $2, NULL, $3::jsonb, '[]'::jsonb, '{}'::jsonb, now())
RETURNING id;
`
	var id string
	return db.QueryRowContext(ctx, insertSQL, userID, projectIDVal, string(reqJSON)).Scan(&id)
}

func (h *SuggestHandler) HandleSuggest(c *gin.Context) {
	var req SuggestRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.ProjectID == "" || req.RunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and run_id are required"})
		return
	}

	if req.UserID != "" {
		ctxCheck, cancelCheck := context.WithTimeout(c.Request.Context(), 5*time.Second)
		hasDesign, err := hasDesignForProject(ctxCheck, h.db, req.UserID, req.ProjectID)
		cancelCheck()
		if err != nil {
			log.Printf("check design for project: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify design: " + err.Error()})
			return
		}
		if !hasDesign && isZeroDesign(req.Design) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "no design found for this project; call POST /analysis-suggestions/design with this project_id first, or send design in the suggest body",
			})
			return
		}
		if !hasDesign && !isZeroDesign(req.Design) {
			ctxEnsure, cancelEnsure := context.WithTimeout(c.Request.Context(), 5*time.Second)
			if err := ensureDesignForProject(ctxEnsure, h.db, req.UserID, req.ProjectID, req.Design); err != nil {
				cancelEnsure()
				log.Printf("ensure design for project: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store design: " + err.Error()})
				return
			}
			cancelEnsure()
		}
	}

	if isZeroDesign(req.Design) && req.UserID != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		if d, err := loadLatestDesign(ctx, h.db, req.UserID, req.ProjectID); err == nil && !isZeroDesign(d) {
			req.Design = d
		}
	}

	ruleFile := h.rulePath
	if req.RuleFile != "" {
		ruleFile = req.RuleFile
	}
	engine, err := rules.NewEngineFromFile(ruleFile, h.db)
	if err != nil {
		log.Printf("failed to load rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load rules: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	results, storageID, err := engine.EvaluateAndStore(ctx, req.UserID, req.ProjectID, req.RunID, req.Design, req.Simulation, req.Candidates)
	if err != nil {
		log.Printf("evaluation/store error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "evaluation/store failed: " + err.Error()})
		return
	}

	if len(results) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "no candidates provided", "storage_id": storageID})
		return
	}

	resp := SuggestResponse{
		Best:      results[0],
		AllScores: results,
		StorageID: storageID,
	}
	c.JSON(http.StatusOK, resp)
}
