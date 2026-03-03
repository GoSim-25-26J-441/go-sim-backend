package http

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Register registers all analysis-suggestions routes on the given router group
func Register(rg *gin.RouterGroup, db *sql.DB, redisClient *redis.Client, rulesPath string) {
	fetchHandler := NewFetchHandler()
	fetchHandler.RegisterRoutes(rg)

	importHandler := NewImportHandler()
	importHandler.RegisterRoutes(rg)

	suggestHandler := NewSuggestHandler(rulesPath, db)
	suggestHandler.RegisterRoutes(rg)

	costHandler := NewCostHandler(db, redisClient)
	costHandler.RegisterRoutes(rg)

	reqHandler := NewRequestHandler(db)
	reqHandler.RegisterRoutes(rg)
}
