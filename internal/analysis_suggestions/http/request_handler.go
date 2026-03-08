package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type RequestResponseRow struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	ProjectID     string          `json:"project_id,omitempty"`
	RunID         string          `json:"run_id,omitempty"`
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
	rg.GET("/requests/:id", h.GetRequestByID)
	rg.GET("/requests/by-project-run", h.GetLatestByProjectRun)
	rg.POST("/design", h.CreateDesignRequest)
}

func (h *RequestHandler) GetRequestsByUser(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	query := `
SELECT id, user_id, COALESCE(project_id,''), COALESCE(run_id,''), request, response, best_candidate, created_at
FROM request_responses
WHERE user_id = $1
ORDER BY project_id, created_at DESC
`

	rows, err := h.db.QueryContext(ctx, query, userID)
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

		if err := rows.Scan(&r.ID, &r.UserID, &r.ProjectID, &r.RunID, &requestBytes, &responseBytes, &bestBytes, &r.CreatedAt); err != nil {
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

func (h *RequestHandler) GetRequestByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	queryByID := `
SELECT id, user_id, COALESCE(project_id,''), COALESCE(run_id,''), request, response, best_candidate, created_at
FROM request_responses
WHERE id = $1
`
	var r RequestResponseRow
	var requestBytes []byte
	var responseBytes []byte
	var bestBytes []byte

	err := h.db.QueryRowContext(ctx, queryByID, id).Scan(&r.ID, &r.UserID, &r.ProjectID, &r.RunID, &requestBytes, &responseBytes, &bestBytes, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed: " + err.Error()})
		return
	}

	r.Request = json.RawMessage(requestBytes)
	r.Response = json.RawMessage(responseBytes)
	r.BestCandidate = json.RawMessage(bestBytes)

	c.JSON(http.StatusOK, r)
}

type CreateDesignRequestBody struct {
	UserID    string          `json:"user_id"`
	ProjectID string          `json:"project_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	Design    json.RawMessage `json:"design"`
}

func (h *RequestHandler) CreateDesignRequest(c *gin.Context) {
	var body CreateDesignRequestBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if body.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	reqEnvelope := map[string]any{
		"design":     json.RawMessage(body.Design),
		"simulation": map[string]any{"nodes": 0},
		"candidates": []any{},
	}
	reqJSON, err := json.Marshal(reqEnvelope)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to marshal request: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	const insertSQL = `
INSERT INTO request_responses (user_id, project_id, run_id, request, response, best_candidate, created_at)
VALUES ($1, $2, $3, $4::jsonb, '[]'::jsonb, '{}'::jsonb, now())
RETURNING id;
`

	var id string
	projectIDVal, runIDVal := interface{}(body.ProjectID), interface{}(body.RunID)
	if body.ProjectID == "" {
		projectIDVal = nil
	}
	if body.RunID == "" {
		runIDVal = nil
	}

	if err := h.db.QueryRowContext(ctx, insertSQL, body.UserID, projectIDVal, runIDVal, string(reqJSON)).Scan(&id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *RequestHandler) GetLatestByProjectRun(c *gin.Context) {
	userID := c.Query("user_id")
	projectID := c.Query("project_id")
	runID := c.Query("run_id")

	if userID == "" || projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id and project_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	const queryWithRun = `
SELECT id, user_id, COALESCE(project_id,''), COALESCE(run_id,''), request, response, best_candidate, created_at
FROM request_responses
WHERE user_id = $1 AND project_id = $2 AND run_id = $3
ORDER BY created_at DESC
LIMIT 1;
`

	const queryWithoutRun = `
SELECT id, user_id, COALESCE(project_id,''), COALESCE(run_id,''), request, response, best_candidate, created_at
FROM request_responses
WHERE user_id = $1 AND project_id = $2
ORDER BY created_at DESC
LIMIT 1;
`

	var r RequestResponseRow
	var requestBytes []byte
	var responseBytes []byte
	var bestBytes []byte

	var err error
	if runID != "" {
		err = h.db.QueryRowContext(ctx, queryWithRun, userID, projectID, runID).
			Scan(&r.ID, &r.UserID, &r.ProjectID, &r.RunID, &requestBytes, &responseBytes, &bestBytes, &r.CreatedAt)
	} else {
		err = h.db.QueryRowContext(ctx, queryWithoutRun, userID, projectID).
			Scan(&r.ID, &r.UserID, &r.ProjectID, &r.RunID, &requestBytes, &responseBytes, &bestBytes, &r.CreatedAt)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if runID != "" {
				c.JSON(http.StatusNotFound, gin.H{"error": "no request found for given user/project/run"})
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "no request found for given user/project"})
			}
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed: " + err.Error()})
		return
	}

	if runID == "" {
		var reqEnvelope map[string]json.RawMessage
		if err := json.Unmarshal(requestBytes, &reqEnvelope); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse request payload: " + err.Error()})
			return
		}

		design, ok := reqEnvelope["design"]
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "design field not found in request payload"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user_id":    r.UserID,
			"project_id": r.ProjectID,
			"design":     json.RawMessage(design),
		})
		return
	}

	r.Request = json.RawMessage(requestBytes)
	r.Response = json.RawMessage(responseBytes)
	r.BestCandidate = json.RawMessage(bestBytes)

	c.JSON(http.StatusOK, r)
}
