package analysis_suggestions

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

type CostHandler struct {
	db    *sql.DB
	redis *redis.Client
}

func NewCostHandler(db *sql.DB, redisClient *redis.Client) *CostHandler {
	return &CostHandler{
		db:    db,
		redis: redisClient,
	}
}

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/cost/:id", h.HandleCost)
	rg.GET("/cost/regions/:provider", h.GetProviderRegions)
	rg.POST("/cost/:id/provider/:provider", h.HandleCostForProvider)
}

// GET REGIONS
func (h *CostHandler) GetProviderRegions(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	db := h.db

	cacheKey := fmt.Sprintf("analysis:regions:%s", provider)
	if h.redis != nil {
		if cached, err := h.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var cachedRegions []string
			if err := json.Unmarshal([]byte(cached), &cachedRegions); err == nil {
				c.JSON(http.StatusOK, gin.H{
					"provider": provider,
					"regions":  cachedRegions,
				})
				return
			}
			log.Printf("redis: failed to unmarshal regions cache for provider %s: %v", provider, err)
		}
	}

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

	if h.redis != nil {
		if b, err := json.Marshal(list); err == nil {
			if err := h.redis.Set(ctx, cacheKey, b, 6*time.Hour).Err(); err != nil {
				log.Printf("redis: failed to cache regions for provider %s: %v", provider, err)
			}
		} else {
			log.Printf("redis: failed to marshal regions for provider %s: %v", provider, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"regions":  list,
	})
}

// Calculation for all providers
func (h *CostHandler) HandleCost(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()
	db := h.db

	filterProvider := strings.ToLower(c.Query("provider"))
	filterRegion := c.Query("region")

	if h.redis != nil {
		cacheKey := fmt.Sprintf("analysis:cost:all:%s:provider=%s:region=%s", id, filterProvider, filterRegion)
		if cached, err := h.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var cachedResp CostResponse
			if err := json.Unmarshal([]byte(cached), &cachedResp); err == nil {
				c.JSON(http.StatusOK, cachedResp)
				return
			}
			log.Printf("redis: failed to unmarshal cached cost response for id=%s: %v", id, err)
		}
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

	if h.redis != nil {
		cacheKey := fmt.Sprintf("analysis:cost:all:%s:provider=%s:region=%s", id, filterProvider, filterRegion)
		if b, err := json.Marshal(resp); err == nil {
			if err := h.redis.Set(ctx, cacheKey, b, 6*time.Hour).Err(); err != nil {
				log.Printf("redis: failed to cache cost response for id=%s: %v", id, err)
			}
		} else {
			log.Printf("redis: failed to marshal cost response for id=%s: %v", id, err)
		}
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

	ctx, cancel := context.WithTimeout(c, 15*time.Second)
	defer cancel()
	db := h.db

	if h.redis != nil {
		cacheKey := fmt.Sprintf("analysis:cost:provider:%s:%s:%s", id, provider, region)
		if cached, err := h.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var cachedResp CostResponse
			if err := json.Unmarshal([]byte(cached), &cachedResp); err == nil {
				c.JSON(http.StatusOK, cachedResp)
				return
			}
			log.Printf("redis: failed to unmarshal cached provider cost response for id=%s: %v", id, err)
		}
	}

	// Read best_candidate JSON
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

	// Calculate cluster costs for specific provider and region
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

	if h.redis != nil {
		cacheKey := fmt.Sprintf("analysis:cost:provider:%s:%s:%s", id, provider, region)
		if b, err := json.Marshal(resp); err == nil {
			if err := h.redis.Set(ctx, cacheKey, b, 6*time.Hour).Err(); err != nil {
				log.Printf("redis: failed to cache provider cost response for id=%s: %v", id, err)
			}
		} else {
			log.Printf("redis: failed to marshal provider cost response for id=%s: %v", id, err)
		}
	}

	c.JSON(http.StatusOK, resp)
}
