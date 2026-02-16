package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat/repository"
)

// ChatService handles chat-related business logic
type ChatService struct {
	repo *repository.ChatRepository
	llm  *chat.LLMClient
}

// NewChatService creates a new chat service
func NewChatService(repo *repository.ChatRepository, llm *chat.LLMClient) *ChatService {
	return &ChatService{
		repo: repo,
		llm:  llm,
	}
}

// CreateThread creates a new chat thread
func (s *ChatService) CreateThread(ctx context.Context, userID, publicID string, title *string, bindingMode string) (*domain.Thread, error) {
	return s.repo.CreateThread(ctx, userID, publicID, title, bindingMode)
}

// ListThreads lists all threads for a project
func (s *ChatService) ListThreads(ctx context.Context, userID, publicID string) ([]domain.Thread, error) {
	return s.repo.ListThreads(ctx, userID, publicID)
}

// PostMessageRequest contains the request data for posting a message
type PostMessageRequest struct {
	Message             string
	Mode                string
	Detail              string
	ForceLLM            bool
	DiagramVersionID    *string
	RequirementsAnswers map[string]interface{}
	Attachments         []AttachmentInput
}

// AttachmentInput represents an attachment in the request
type AttachmentInput struct {
	ObjectKey string
	MimeType  *string
	FileName  *string
	FileSize  *int64
	Width     *int
	Height    *int
}

// PostMessageResponse contains the response data after posting a message
type PostMessageResponse struct {
	Answer               string
	Source               string
	Refs                 []string
	DiagramVersionIDUsed *string
	UserMessage          *domain.Message
	AssistantMessage     *domain.Message
}

// UpdateThreadBinding updates a thread's binding mode and pinned diagram.
func (s *ChatService) UpdateThreadBinding(
	ctx context.Context,
	userID, publicID, threadID string,
	bindingMode string,
	diagramVersionID *string,
) (*domain.Thread, error) {
	return s.repo.UpdateThreadBinding(ctx, userID, publicID, threadID, bindingMode, diagramVersionID)
}

// isMeaningfulJSON checks if json.RawMessage contains meaningful data
func isMeaningfulJSON(data json.RawMessage) bool {
	if len(data) == 0 {
		return false
	}

	// Trim whitespace and convert to string for comparison
	trimmed := strings.TrimSpace(string(data))

	// Check for empty, {}, or null
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return false
	}

	return true
}

// Returns the same diagram_json with metadata set; if injection fails, returns the original.
func injectDiagramMetadata(diagramJSON json.RawMessage, diagramVersionID *string) json.RawMessage {
	if diagramVersionID == nil || *diagramVersionID == "" {
		return diagramJSON
	}
	var m map[string]interface{}
	if err := json.Unmarshal(diagramJSON, &m); err != nil {
		return diagramJSON
	}
	if m == nil {
		m = make(map[string]interface{})
	}
	meta, _ := m["metadata"].(map[string]interface{})
	if meta == nil {
		meta = make(map[string]interface{})
	}
	meta["diagram_version_id"] = *diagramVersionID
	m["metadata"] = meta
	out, err := json.Marshal(m)
	if err != nil {
		return diagramJSON
	}
	return out
}

// PostMessage posts a message to a thread and gets an LLM response
func (s *ChatService) PostMessage(ctx context.Context, userID, publicID, threadID string, req PostMessageRequest) (*PostMessageResponse, error) {
	var diagramVersionIDUsed *string
	var specSummary json.RawMessage
	var diagramJSON json.RawMessage
	var err error

	// If diagram_version_id is provided, use that specific version (override)
	if req.DiagramVersionID != nil && *req.DiagramVersionID != "" {
		specSummary, diagramJSON, err = s.repo.GetDiagramVersionByID(ctx, userID, publicID, *req.DiagramVersionID)
		if err != nil {
			return nil, fmt.Errorf("get diagram version: %w", err)
		}
		diagramVersionIDUsed = req.DiagramVersionID
	} else {
		// Otherwise, resolve diagram context from thread binding (FOLLOW_LATEST or PINNED)
		diagramVersionIDUsed, specSummary, diagramJSON, err = s.repo.ResolveDiagramContext(ctx, userID, publicID, threadID)
		if err != nil {
			return nil, fmt.Errorf("resolve diagram context: %w", err)
		}
	}

	// Get chat history
	roles, contents, err := s.repo.ListHistoryForUIGP(ctx, userID, publicID, threadID, 20)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}

	// Reverse to chronological order
	history := make([]chat.ChatMessage, 0, len(roles))
	for i := len(roles) - 1; i >= 0; i-- {
		history = append(history, chat.ChatMessage{
			Role:    roles[i],
			Content: contents[i],
		})
	}

	// Build requirements summary if this is the first message (no history)
	userMessage := req.Message
	if len(history) == 0 {
		if len(req.RequirementsAnswers) > 0 {
			// Requirements answers provided - build summary
			summary := rag.BuildRequirementsSummary(req.RequirementsAnswers)
			if summary != "" {
				// Prepend the requirements summary to the user message
				userMessage = summary + "\n\n" + req.Message
			}
		} else {
			// No requirements answers provided - add note
			userMessage = "Note: No requirements_answers available. " + req.Message
		}
	}

	// Map attachments to API format
	attachments := make([]chat.AttachmentRequest, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		if strings.TrimSpace(a.ObjectKey) == "" {
			continue
		}

		name := a.ObjectKey
		if a.FileName != nil && *a.FileName != "" {
			name = *a.FileName
		}

		kind := "diagram"
		if a.MimeType != nil {
			if strings.HasPrefix(*a.MimeType, "image/") {
				kind = "diagram"
			} else if strings.Contains(*a.MimeType, "word") || strings.Contains(*a.MimeType, "document") {
				kind = "requirements"
			}
		}

		attachments = append(attachments, chat.AttachmentRequest{
			Name:        name,
			ContentType: getStringValue(a.MimeType),
			SizeBytes:   a.FileSize,
			SHA256:      "", // Optional, can be empty for now
			Kind:        kind,
		})
	}

	// Build LLM request
	mode := strings.TrimSpace(req.Mode)
	detail := strings.TrimSpace(req.Detail)
	hasDiagramContext := isMeaningfulJSON(diagramJSON) || isMeaningfulJSON(specSummary)

	// When we send diagram context, UIGP expects mode "thinking" and detail "high" to use it
	if hasDiagramContext {
		if mode == "" || mode == "default" {
			mode = "thinking"
		}
		if detail == "" {
			detail = "high"
		}
	}

	llmReq := chat.ChatRequest{
		Message:     userMessage,
		History:     history,
		Mode:        mode,
		Detail:      detail,
		Attachments: attachments,
	}

	// Only include spec_summary if it has meaningful content
	if isMeaningfulJSON(specSummary) {
		llmReq.SpecSummary = specSummary
	}

	// Include diagram_json in UIGP format: add metadata.diagram_version_id when we have a version
	if isMeaningfulJSON(diagramJSON) {
		llmReq.DiagramJSON = injectDiagramMetadata(diagramJSON, diagramVersionIDUsed)
	}

	// Call LLM client
	llmResp, err := s.llm.Chat(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	// Extract source string from SourceInfo
	sourceStr := llmResp.Source.Provider
	if llmResp.Source.Model != "" {
		sourceStr = fmt.Sprintf("%s/%s", llmResp.Source.Provider, llmResp.Source.Model)
	}

	// Map attachments for database
	dbAtts := make([]repository.InsertAttachment, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		if strings.TrimSpace(a.ObjectKey) == "" {
			continue
		}
		dbAtts = append(dbAtts, repository.InsertAttachment{
			ObjectKey: strings.TrimSpace(a.ObjectKey),
			MimeType:  a.MimeType,
			FileName:  a.FileName,
			FileSize:  a.FileSize,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	// Insert turn into database
	// Use the original message (not the one with requirements summary prepended) for storage
	uMsg, aMsg, err := s.repo.InsertTurn(
		ctx,
		userID, publicID, threadID,
		req.Message,
		llmResp.Answer,
		&sourceStr,
		llmResp.Refs,
		diagramVersionIDUsed,
		dbAtts,
	)
	if err != nil {
		return nil, fmt.Errorf("insert turn: %w", err)
	}

	// If diagram_version_id was provided, update thread binding to PINNED with that version
	// This ensures future messages in the thread use the same diagram version
	if req.DiagramVersionID != nil && *req.DiagramVersionID != "" {
		_, err = s.repo.UpdateThreadBinding(ctx, userID, publicID, threadID, domain.BindingPinned, req.DiagramVersionID)
		if err != nil {
			// Log error but don't fail the request - message was already saved
			// In production, use proper logging here
			_ = err
		}
	}

	return &PostMessageResponse{
		Answer:               llmResp.Answer,
		Source:               sourceStr,
		Refs:                 llmResp.Refs,
		DiagramVersionIDUsed: diagramVersionIDUsed,
		UserMessage:          uMsg,
		AssistantMessage:     aMsg,
	}, nil
}

// ListMessages lists messages in a thread
func (s *ChatService) ListMessages(ctx context.Context, userID, publicID, threadID string, limit int) ([]domain.Message, error) {
	return s.repo.ListMessages(ctx, userID, publicID, threadID, limit)
}

// Helper function to safely get string value from pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
