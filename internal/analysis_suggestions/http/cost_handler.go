package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
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

type GlobalRecommendResponse struct {
	RequestID      string                  `json:"request_id"`
	BestCandidate  rules.CandidateScore    `json:"best_candidate"`
	NodeCount      int                     `json:"nodes"`
	Budget         float64                 `json:"budget"`
	Recommendation cc.GlobalRecommendation `json:"recommendation"`
	StoredAt       string                  `json:"stored_at,omitempty"`
}

const (
	costCacheKeyPrefix          = "analysis:cost:"
	costRecommendCacheKeyPrefix = "analysis:cost:recommend:"
	costCacheTTL                = 10 * time.Minute
)

type globalCostRecommendationRow struct {
	FitsBudget          bool                   `json:"fits_budget"`
	Recommended         *cc.ClusterCostResult  `json:"recommended,omitempty"`
	RunnersUp           []cc.ClusterCostResult `json:"runners_up"`
	Rationale           []string               `json:"rationale"`
	RegionJobsEvaluated int                    `json:"region_jobs_evaluated"`
	PlansEvaluated      int                    `json:"plans_evaluated"`
	WithinBudgetPlans   int                    `json:"within_budget_plans"`
	ComputedAt          string                 `json:"computed_at"`
}

func globalRecForDB(rec *cc.GlobalRecommendation) globalCostRecommendationRow {
	return globalCostRecommendationRow{
		FitsBudget:          rec.FitsBudget,
		Recommended:         rec.Recommended,
		RunnersUp:           rec.RunnersUp,
		Rationale:           rec.Rationale,
		RegionJobsEvaluated: rec.RegionJobsEvaluated,
		PlansEvaluated:      rec.PlansEvaluated,
		WithinBudgetPlans:   rec.WithinBudgetPlans,
		ComputedAt:          time.Now().UTC().Format(time.RFC3339),
	}
}

func (h *CostHandler) persistGlobalCostRecommendation(ctx context.Context, requestID string, rec *cc.GlobalRecommendation) {
	payload := globalRecForDB(rec)
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal global recommendation for persist: %v", err)
		return
	}
	_, err = h.db.ExecContext(ctx, `
		UPDATE request_responses SET global_cost_recommendation = $1::jsonb WHERE id = $2::uuid
	`, b, requestID)
	if err != nil {
		log.Printf("persist global recommendation for %s: %v", requestID, err)
	}
}

type CostHandler struct {
	db          *sql.DB
	redisClient *redis.Client
}

func nearlyWholeDivision(v float64, divisor int) bool {
	if divisor <= 0 {
		return false
	}
	q := v / float64(divisor)
	return math.Abs(q-math.Round(q)) < 1e-6
}

// normalizeBestCandidatePerNode converts cluster-total specs to per-node specs
// when historical rows stored totals instead of instance sizing.
func normalizeBestCandidatePerNode(best rules.CandidateScore, nodes, preferredVCPU int, preferredMemoryGB float64) rules.CandidateScore {
	if nodes <= 1 {
		return best
	}

	spec := best.Candidate.Spec
	if spec.VCPU <= 0 && spec.MemoryGB <= 0 {
		return best
	}

	cpuLooksClusterTotal := false
	memLooksClusterTotal := false

	if spec.VCPU > 0 && spec.VCPU%nodes == 0 {
		perNodeCPU := spec.VCPU / nodes
		if preferredVCPU > 0 {
			cpuLooksClusterTotal = perNodeCPU == preferredVCPU || spec.VCPU == preferredVCPU*nodes
		}
	}

	if spec.MemoryGB > 0 && nearlyWholeDivision(spec.MemoryGB, nodes) {
		perNodeMem := spec.MemoryGB / float64(nodes)
		if preferredMemoryGB > 0 {
			memLooksClusterTotal = math.Abs(perNodeMem-preferredMemoryGB) < 1e-6 ||
				math.Abs(spec.MemoryGB-(preferredMemoryGB*float64(nodes))) < 1e-6
		}
	}

	// Require at least one strong signal from design context before converting.
	if !cpuLooksClusterTotal && !memLooksClusterTotal {
		return best
	}

	if spec.VCPU > 0 && spec.VCPU%nodes == 0 {
		best.Candidate.Spec.VCPU = spec.VCPU / nodes
	}
	if spec.MemoryGB > 0 {
		best.Candidate.Spec.MemoryGB = spec.MemoryGB / float64(nodes)
	}
	return best
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

func (h *CostHandler) recommendCacheKey(id string) string {
	return costRecommendCacheKeyPrefix + id
}

func (h *CostHandler) getCachedGlobalRecommend(ctx context.Context, key string) (*GlobalRecommendResponse, bool) {
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
	var resp GlobalRecommendResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		log.Printf("failed to unmarshal cached global recommend for key %s: %v", key, err)
		return nil, false
	}
	return &resp, true
}

func (h *CostHandler) setCachedGlobalRecommend(ctx context.Context, key string, resp *GlobalRecommendResponse) {
	if h.redisClient == nil {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("failed to marshal global recommend for cache key %s: %v", key, err)
		return
	}
	if err := h.redisClient.Set(ctx, key, data, costCacheTTL).Err(); err != nil {
		log.Printf("failed to set global recommend cache for key %s: %v", key, err)
	}
}

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/cost/:id", h.HandleCost)
	rg.POST("/cost/:id/recommend", h.HandleGlobalRecommend)
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

	var bestJSON, reqJSON []byte
	if err := db.QueryRowContext(ctx, `SELECT best_candidate::text, request::text FROM request_responses WHERE id = $1`, id).Scan(&bestJSON, &reqJSON); err != nil {
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

	var req struct {
		Design struct {
			PreferredVCPU     int     `json:"preferred_vcpu"`
			PreferredMemoryGB float64 `json:"preferred_memory_gb"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}
	bestCS = normalizeBestCandidatePerNode(
		bestCS,
		req.Simulation.Nodes,
		req.Design.PreferredVCPU,
		req.Design.PreferredMemoryGB,
	)
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
			Budget            float64 `json:"budget"`
			PreferredVCPU     int     `json:"preferred_vcpu"`
			PreferredMemoryGB float64 `json:"preferred_memory_gb"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}
	bestCS = normalizeBestCandidatePerNode(
		bestCS,
		req.Simulation.Nodes,
		req.Design.PreferredVCPU,
		req.Design.PreferredMemoryGB,
	)

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

func (h *CostHandler) HandleGlobalRecommend(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c, 60*time.Second)
	defer cancel()
	db := h.db

	cacheKey := h.recommendCacheKey(id)
	if cached, ok := h.getCachedGlobalRecommend(ctx, cacheKey); ok {
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
			Budget            float64 `json:"budget"`
			PreferredVCPU     int     `json:"preferred_vcpu"`
			PreferredMemoryGB float64 `json:"preferred_memory_gb"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}
	bestCS = normalizeBestCandidatePerNode(
		bestCS,
		req.Simulation.Nodes,
		req.Design.PreferredVCPU,
		req.Design.PreferredMemoryGB,
	)

	rec, err := cc.BuildGlobalRecommendation(ctx, db, bestCS, req.Simulation.Nodes, req.Design.Budget)
	if err != nil {
		log.Printf("global recommend failed for %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build global recommendation"})
		return
	}

	resp := GlobalRecommendResponse{
		RequestID:      id,
		BestCandidate:  bestCS,
		NodeCount:      req.Simulation.Nodes,
		Budget:         req.Design.Budget,
		Recommendation: *rec,
		StoredAt:       created.UTC().Format(time.RFC3339),
	}

	h.persistGlobalCostRecommendation(ctx, id, rec)
	h.setCachedGlobalRecommend(ctx, cacheKey, &resp)
	c.JSON(http.StatusOK, &resp)
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
			Budget            float64 `json:"budget"`
			PreferredVCPU     int     `json:"preferred_vcpu"`
			PreferredMemoryGB float64 `json:"preferred_memory_gb"`
		} `json:"design"`
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse request"})
		return
	}
	bestCS = normalizeBestCandidatePerNode(
		bestCS,
		req.Simulation.Nodes,
		req.Design.PreferredVCPU,
		req.Design.PreferredMemoryGB,
	)

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
