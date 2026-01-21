package chats

import (
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	repo *Repo
	uigp *dipllm.UIGPClient
}

func NewHandler(repo *Repo, uigp *dipllm.UIGPClient) *Handler {
	return &Handler{repo: repo, uigp: uigp}
}

type createThreadReq struct {
	Title       *string `json:"title"`
	BindingMode string  `json:"binding_mode,omitempty"`
}

func (h *Handler) createThread(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	userID := auth.UserFirebaseUID(c)
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	var req createThreadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	t, err := h.repo.CreateThread(c.Request.Context(), userID, publicID, req.Title, req.BindingMode)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "thread": t})
}

func (h *Handler) listThreads(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	userID := auth.UserFirebaseUID(c)

	items, err := h.repo.ListThreads(c.Request.Context(), userID, publicID)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "threads": items})
}

type postMsgReq struct {
	Message     string `json:"message"`
	Mode        string `json:"mode,omitempty"`
	ForceLLM    bool   `json:"force_llm,omitempty"`
	Attachments []struct {
		ObjectKey string  `json:"object_key"`
		MimeType  *string `json:"mime_type,omitempty"`
		FileName  *string `json:"file_name,omitempty"`
		FileSize  *int64  `json:"file_size_bytes,omitempty"`
		Width     *int    `json:"width,omitempty"`
		Height    *int    `json:"height,omitempty"`
	} `json:"attachments,omitempty"`
}

func (h *Handler) postMessage(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	threadID := strings.TrimSpace(c.Param("thread_id"))
	userID := auth.UserFirebaseUID(c)

	var req postMsgReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	diagramVersionIDUsed, spec, err := h.repo.ResolveDiagramContext(c.Request.Context(), userID, publicID, threadID)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project/thread/diagram not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// history
	roles, contents, err := h.repo.ListHistoryForUIGP(c.Request.Context(), userID, publicID, threadID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// reverse to chronological
	history := make([]dipllm.ChatMessage, 0, len(roles))
	for i := len(roles) - 1; i >= 0; i-- {
		history = append(history, dipllm.ChatMessage{Role: roles[i], Content: contents[i]})
	}

	out, err := h.uigp.Chat(c.Request.Context(), dipllm.ChatRequest{
		SpecSummary: spec,
		History:     history,
		Message:     req.Message,
		Mode:        req.Mode,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// map attachments
	atts := make([]InsertAttachment, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		if strings.TrimSpace(a.ObjectKey) == "" {
			continue
		}
		atts = append(atts, InsertAttachment{
			ObjectKey: strings.TrimSpace(a.ObjectKey),
			MimeType:  a.MimeType,
			FileName:  a.FileName,
			FileSize:  a.FileSize,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	source := out.Source
	uMsg, aMsg, err := h.repo.InsertTurn(
		c.Request.Context(),
		userID, publicID, threadID,
		req.Message,
		out.Answer,
		&source,
		out.Refs,
		diagramVersionIDUsed,
		atts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":                      true,
		"answer":                  out.Answer,
		"source":                  out.Source,
		"refs":                    out.Refs,
		"diagram_version_id_used": diagramVersionIDUsed,
		"user_message":            uMsg,
		"assistant_message":       aMsg,
	})
}

func (h *Handler) listMessages(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	threadID := strings.TrimSpace(c.Param("thread_id"))
	userID := auth.UserFirebaseUID(c)

	items, err := h.repo.ListMessages(c.Request.Context(), userID, publicID, threadID, 50)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project/thread not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "messages": items})
}
