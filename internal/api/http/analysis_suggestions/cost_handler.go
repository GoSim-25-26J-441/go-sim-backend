package analysis_suggestions

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	cc "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/costcal"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CostResponse struct {
	RequestID        string                            `json:"request_id"`
	BestCandidate    rules.CandidateScore              `json:"best_candidate"`
	NodeCount        int                               `json:"nodes"`
	Budget           float64                           `json:"budget"`
	ProviderClusters map[string][]cc.ClusterCostResult `json:"cluster_costs"`
	StoredAt         string                            `json:"stored_at,omitempty"`
}

type CostHandler struct{}

func NewCostHandler() *CostHandler { return &CostHandler{} }

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/cost/:id", h.HandleCost)
	rg.GET("/cost/regions/:provider", h.GetProviderRegions)
	rg.POST("/cost/:id/provider/:provider", h.HandleCostForProvider)
}

// GET REGIONS
func (h *CostHandler) GetProviderRegions(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))

	dbURL := os.Getenv("DATABASE_URL")
	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection failed"})
		return
	}
	defer pool.Close()

	table := map[string]string{
		"aws":   "aws_compute_prices",
		"azure": "azure_compute_prices",
		"gcp":   "gcp_compute_prices",
	}[provider]

	if table == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid provider"})
		return
	}

	rows, err := pool.Query(ctx, `SELECT DISTINCT region FROM `+table+` WHERE region IS NOT NULL ORDER BY region`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch regions"})
		return
	}
	defer rows.Close()

	list := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan region"})
			return
		}
		list = append(list, r)
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"regions":  list,
	})
}

// Calculation for all providers
func (h *CostHandler) HandleCost(c *gin.Context) {
	id := c.Param("id")

	dbURL := os.Getenv("DATABASE_URL")
	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection failed"})
		return
	}
	defer pool.Close()

	var bestJSON, reqJSON []byte
	var created time.Time

	err = pool.QueryRow(ctx, `
		SELECT best_candidate::text, request::text, created_at 
		FROM request_responses WHERE id=$1
	`, id).Scan(&bestJSON, &reqJSON, &created)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	bestCS, err := cc.CandidateScoreFromJSONBytes(bestJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse candidate"})
		return
	}

	var req struct {
		Design struct {
			Budget float64 `json:"budget"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}

	filterProvider := strings.ToLower(c.Query("provider"))
	filterRegion := c.Query("region")

	clusterCosts, err := cc.CalculateClusterCosts(
		ctx,
		pool,
		bestCS,
		req.Simulation.Nodes,
		filterRegion,
		req.Design.Budget,
		filterProvider,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate costs"})
		return
	}

	resp := CostResponse{
		RequestID:        id,
		BestCandidate:    bestCS,
		NodeCount:        req.Simulation.Nodes,
		Budget:           req.Design.Budget,
		ProviderClusters: clusterCosts,
		StoredAt:         created.UTC().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CostHandler) HandleCostForProvider(c *gin.Context) {
	id := c.Param("id")
	provider := strings.ToLower(c.Param("provider"))

	region := c.Query("region")
	if region == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Region parameter is required"})
		return
	}

	dbURL := os.Getenv("DATABASE_URL")
	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection failed"})
		return
	}
	defer pool.Close()

	// Read best_candidate JSON
	var bestJSON, reqJSON []byte
	var created time.Time

	err = pool.QueryRow(ctx, `
		SELECT best_candidate::text, request::text, created_at 
		FROM request_responses WHERE id=$1
	`, id).Scan(&bestJSON, &reqJSON, &created)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	bestCS, err := cc.CandidateScoreFromJSONBytes(bestJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse candidate"})
		return
	}

	var req struct {
		Design struct {
			Budget float64 `json:"budget"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}

	// Calculate cluster costs for specific provider and region
	clusterCosts, err := cc.CalculateClusterCostsForProvider(
		ctx,
		pool,
		provider,
		bestCS,
		req.Simulation.Nodes,
		region,
		req.Design.Budget,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate costs"})
		return
	}

	//Response
	providerClusters := make(map[string][]cc.ClusterCostResult)
	providerClusters[provider] = clusterCosts

	resp := CostResponse{
		RequestID:        id,
		BestCandidate:    bestCS,
		NodeCount:        req.Simulation.Nodes,
		Budget:           req.Design.Budget,
		ProviderClusters: providerClusters,
		StoredAt:         created.UTC().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, resp)
}
