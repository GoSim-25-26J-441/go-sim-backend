package analysis_suggestions

import (
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SuggestRequest struct {
	UserID     string                `json:"user_id,omitempty"`
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
	pool     *pgxpool.Pool
}

func NewSuggestHandler(rulePath string, pool *pgxpool.Pool) *SuggestHandler {
	return &SuggestHandler{rulePath: rulePath, pool: pool}
}

func (h *SuggestHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/suggest", h.HandleSuggest)
}

func (h *SuggestHandler) HandleSuggest(c *gin.Context) {
	var req SuggestRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	ruleFile := h.rulePath
	if req.RuleFile != "" {
		ruleFile = req.RuleFile
	}
	engine, err := rules.NewEngineFromFile(ruleFile, h.pool)
	if err != nil {
		log.Printf("failed to load rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load rules: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	results, storageID, err := engine.EvaluateAndStore(ctx, req.UserID, req.Design, req.Simulation, req.Candidates)
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
