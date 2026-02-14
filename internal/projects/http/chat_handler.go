package http

import (
	"net/http"
	"strings"

	chatdomain "github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat/domain"
	chatservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat/service"
	"github.com/gin-gonic/gin"
)

type createThreadReq struct {
	Title       *string `json:"title"`
	BindingMode string  `json:"binding_mode,omitempty"`
}

func (h *Handler) createThread(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	userID := c.GetString("firebase_uid")
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	var req createThreadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	t, err := h.chatService.CreateThread(c.Request.Context(), userID, publicID, req.Title, req.BindingMode)
	if err != nil {
		if err == chatdomain.ErrNotFound {
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
	userID := c.GetString("firebase_uid")

	items, err := h.chatService.ListThreads(c.Request.Context(), userID, publicID)
	if err != nil {
		if err == chatdomain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "threads": items})
}

type updateThreadBindingReq struct {
	BindingMode      string  `json:"binding_mode"`
	DiagramVersionID *string `json:"diagram_version_id,omitempty"`
}

// updateThreadBinding allows switching a thread between FOLLOW_LATEST and PINNED modes.
func (h *Handler) updateThreadBinding(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	threadID := strings.TrimSpace(c.Param("thread_id"))
	userID := c.GetString("firebase_uid")

	if publicID == "" || threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project or thread id"})
		return
	}

	var req updateThreadBindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	binding := strings.TrimSpace(req.BindingMode)
	if binding == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "binding_mode is required"})
		return
	}

	thread, err := h.chatService.UpdateThreadBinding(c.Request.Context(), userID, publicID, threadID, binding, req.DiagramVersionID)
	if err != nil {
		if err == chatdomain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project/thread/diagram not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "thread": thread})
}

type postMsgReq struct {
	Message          string  `json:"message"`
	Mode             string  `json:"mode,omitempty"`
	Detail           string  `json:"detail,omitempty"`
	ForceLLM         bool    `json:"force_llm,omitempty"`
	DiagramVersionID *string `json:"diagram_version_id,omitempty"`
	Attachments      []struct {
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
	userID := c.GetString("firebase_uid")

	var req postMsgReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	// Map request attachments
	attachments := make([]chatservice.AttachmentInput, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		attachments = append(attachments, chatservice.AttachmentInput{
			ObjectKey: a.ObjectKey,
			MimeType:  a.MimeType,
			FileName:  a.FileName,
			FileSize:  a.FileSize,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	serviceReq := chatservice.PostMessageRequest{
		Message:          req.Message,
		Mode:             req.Mode,
		Detail:           req.Detail,
		ForceLLM:         req.ForceLLM,
		DiagramVersionID: req.DiagramVersionID,
		Attachments:      attachments,
	}

	resp, err := h.chatService.PostMessage(c.Request.Context(), userID, publicID, threadID, serviceReq)
	if err != nil {
		if err == chatdomain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project/thread/diagram not found"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":                      true,
		"answer":                  resp.Answer,
		"source":                  resp.Source,
		"refs":                    resp.Refs,
		"diagram_version_id_used": resp.DiagramVersionIDUsed,
		"user_message":            resp.UserMessage,
		"assistant_message":       resp.AssistantMessage,
	})
}

func (h *Handler) listMessages(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	threadID := strings.TrimSpace(c.Param("thread_id"))
	userID := c.GetString("firebase_uid")

	items, err := h.chatService.ListMessages(c.Request.Context(), userID, publicID, threadID, 50)
	if err != nil {
		if err == chatdomain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project/thread not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "messages": items})
}
