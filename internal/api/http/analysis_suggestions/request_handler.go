package analysis_suggestions

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type RequestResponseRow struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	Request       json.RawMessage `json:"request"`
	Response      json.RawMessage `json:"response"`
	BestCandidate json.RawMessage `json:"best_candidate"`
	CreatedAt     time.Time       `json:"created_at"`
}

type RequestHandler struct {
	db *sql.DB
}

func NewRequestHandler(db *sql.DB) *RequestHandler {
	return &RequestHandler{db: db}
}

func (h *RequestHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/requests/user/:user_id", h.GetRequestsByUser)
}

func (h *RequestHandler) GetRequestsByUser(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	sql := `
SELECT id, user_id, request, response, best_candidate, created_at
FROM request_responses
WHERE user_id = $1
ORDER BY created_at DESC
`

	rows, err := h.db.QueryContext(ctx, sql, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed: " + err.Error()})
		return
	}
	defer rows.Close()

	out := []RequestResponseRow{}
	for rows.Next() {
		var r RequestResponseRow
		var requestBytes []byte
		var responseBytes []byte
		var bestBytes []byte

		if err := rows.Scan(&r.ID, &r.UserID, &requestBytes, &responseBytes, &bestBytes, &r.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db scan failed: " + err.Error()})
			return
		}

		r.Request = json.RawMessage(requestBytes)
		r.Response = json.RawMessage(responseBytes)
		r.BestCandidate = json.RawMessage(bestBytes)

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db rows error: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": userID,
		"count":   len(out),
		"rows":    out,
	})
}
