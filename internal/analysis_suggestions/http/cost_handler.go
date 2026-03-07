package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	cc "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/costcal"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type CostResponse struct {
	RequestID        string                            `json:"request_id"`
	BestCandidate    rules.CandidateScore              `json:"best_candidate"`
	NodeCount        int                               `json:"nodes"`
	Budget           float64                           `json:"budget"`
	ProviderClusters map[string][]cc.ClusterCostResult `json:"cluster_costs"`
	StoredAt         string                            `json:"stored_at,omitempty"`
}

const (
	costCacheKeyPrefix = "analysis:cost:"
	costCacheTTL       = 10 * time.Minute
)

type CostHandler struct {
	db          *sql.DB
	redisClient *redis.Client
}

func NewCostHandler(db *sql.DB, redisClient *redis.Client) *CostHandler {
	return &CostHandler{
		db:          db,
		redisClient: redisClient,
	}
}

func (h *CostHandler) cacheKey(id, provider, region string) string {
	if provider == "" {
		provider = "all"
	}
	if region == "" {
		region = "all"
	}
	return fmt.Sprintf("%s%s:%s:%s", costCacheKeyPrefix, id, provider, region)
}

func (h *CostHandler) getCachedCost(ctx context.Context, key string) (*CostResponse, bool) {
	if h.redisClient == nil {
		return nil, false
	}

	data, err := h.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Printf("redis get error for key %s: %v", key, err)
		}
		return nil, false
	}

	var resp CostResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		log.Printf("failed to unmarshal cached cost for key %s: %v", key, err)
		return nil, false
	}

	return &resp, true
}

func (h *CostHandler) setCachedCost(ctx context.Context, key string, resp *CostResponse) {
	if h.redisClient == nil {
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("failed to marshal cost response for cache key %s: %v", key, err)
		return
	}

	if err := h.redisClient.Set(ctx, key, data, costCacheTTL).Err(); err != nil {
		log.Printf("failed to set cost cache for key %s: %v", key, err)
	}
}

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/cost/:id", h.HandleCost)
	rg.GET("/cost/regions/:provider", h.GetProviderRegions)
	rg.GET("/cost/:id/regions/:provider", h.GetRegionsForRequest)
	rg.POST("/cost/:id/provider/:provider", h.HandleCostForProvider)
}

func (h *CostHandler) GetProviderRegions(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	db := h.db

	table := map[string]string{
		"aws":   "aws_compute_prices",
		"azure": "azure_compute_prices",
		"gcp":   "gcp_compute_prices",
	}[provider]

	if table == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid provider"})
		return
	}

	rows, err := db.QueryContext(ctx, `SELECT DISTINCT region FROM `+table+` WHERE region IS NOT NULL ORDER BY region`)
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

func (h *CostHandler) GetRegionsForRequest(c *gin.Context) {
	id := c.Param("id")
	provider := strings.ToLower(c.Param("provider"))
	if provider != "aws" && provider != "azure" && provider != "gcp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid provider"})
		return
	}
	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	db := h.db

	var bestJSON []byte
	if err := db.QueryRowContext(ctx, `SELECT best_candidate::text FROM request_responses WHERE id = $1`, id).Scan(&bestJSON); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load request"})
		return
	}
	bestCS, err := cc.CandidateScoreFromJSONBytes(bestJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse best candidate"})
		return
	}
	vcpu := bestCS.Candidate.Spec.VCPU
	mem := bestCS.Candidate.Spec.MemoryGB

	list, err := cc.GetRegionsForCandidateSpec(ctx, db, provider, vcpu, mem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch regions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"regions":  list,
	})
}

func (h *CostHandler) HandleCost(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()
	db := h.db

	filterProvider := strings.ToLower(c.Query("provider"))
	filterRegion := c.Query("region")

	cacheKey := h.cacheKey(id, filterProvider, filterRegion)
	if cached, ok := h.getCachedCost(ctx, cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	var bestJSON, reqJSON []byte
	var created time.Time

	err := db.QueryRowContext(ctx, `
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

	clusterCosts, err := cc.CalculateClusterCosts(
		ctx,
		db,
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

	h.setCachedCost(ctx, cacheKey, &resp)

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

	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()
	db := h.db

	cacheKey := h.cacheKey(id, provider, region)
	if cached, ok := h.getCachedCost(ctx, cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	var bestJSON, reqJSON []byte
	var created time.Time

	err := db.QueryRowContext(ctx, `
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

	clusterCosts, err := cc.CalculateClusterCostsForProvider(
		ctx,
		db,
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

	h.setCachedCost(ctx, cacheKey, &resp)

	c.JSON(http.StatusOK, resp)
}
