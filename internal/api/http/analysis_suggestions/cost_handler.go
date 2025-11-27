package analysis_suggestions

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	cc "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/costcal"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CostResponse is returned by the handler
type CostResponse struct {
	RequestID     string                   `json:"request_id"`
	BestCandidate rules.CandidateScore     `json:"best_candidate"`
	ProviderCosts map[string]cc.CostResult `json:"provider_costs"`
	StoredAt      string                   `json:"stored_at,omitempty"`
}

// CostHandler handles GET /api/cost/:id
type CostHandler struct{}

func NewCostHandler() *CostHandler { return &CostHandler{} }

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/cost/:id", h.HandleCost)
}

func (h *CostHandler) HandleCost(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id in path"})
		return
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DATABASE_URL not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Printf("db connect error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db connect failed: " + err.Error()})
		return
	}
	defer pool.Close()

	// Read best_candidate JSON
	var bestJSON []byte
	var createdAt time.Time
	row := pool.QueryRow(ctx, `SELECT best_candidate::text, created_at FROM request_responses WHERE id = $1`, id)
	if err := row.Scan(&bestJSON, &createdAt); err != nil {
		log.Printf("db select error: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found: " + err.Error()})
		return
	}

	bestCS, err := cc.CandidateScoreFromJSONBytes(bestJSON)
	if err != nil {
		log.Printf("json unmarshal best_candidate error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse stored best_candidate: " + err.Error()})
		return
	}

	providerCosts, err := cc.CalculateCostsForBestCandidate(ctx, pool, bestCS)
	if err != nil {
		log.Printf("cost calculation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cost calculation failed: " + err.Error()})
		return
	}

	//Response
	resp := CostResponse{
		RequestID:     id,
		BestCandidate: bestCS,
		ProviderCosts: providerCosts,
		StoredAt:      createdAt.UTC().Format(time.RFC3339),
	}
	c.JSON(http.StatusOK, resp)
}
