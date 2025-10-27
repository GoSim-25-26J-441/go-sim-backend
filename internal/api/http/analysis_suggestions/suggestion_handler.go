package analysis_suggestions

import (
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
)

type SuggestRequest struct {
	Design     rules.DesignInput `json:"design"`
	Candidates []rules.Candidate `json:"candidates"`
	RuleFile   string            `json:"rule_file,omitempty"`
}

type SuggestResponse struct {
	Best       rules.CandidateScore   `json:"best"`
	AllScores  []rules.CandidateScore `json:"all_scores"`
	RuleSource string                 `json:"rule_source"`
}

type SuggestHandler struct {
	rulePath string
}

func NewSuggestHandler(rulePath string) *SuggestHandler {
	return &SuggestHandler{rulePath: rulePath}
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
	engine, err := rules.NewEngineFromFile(ruleFile)
	if err != nil {
		log.Printf("failed to load rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load rules: " + err.Error()})
		return
	}
	scores, err := engine.EvaluateCandidates(req.Design, req.Candidates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "evaluation failed: " + err.Error()})
		return
	}
	if len(scores) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "no candidates provided"})
		return
	}
	res := SuggestResponse{
		Best:       scores[0],
		AllScores:  scores,
		RuleSource: ruleFile,
	}
	c.JSON(http.StatusOK, res)
}
