package service

import (
	"context"
	"fmt"
	"strings"

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
	Message     string
	Mode        string
	Detail      string
	ForceLLM    bool
	Attachments []AttachmentInput
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

// PostMessage posts a message to a thread and gets an LLM response
func (s *ChatService) PostMessage(ctx context.Context, userID, publicID, threadID string, req PostMessageRequest) (*PostMessageResponse, error) {
	// Resolve diagram context - now returns diagram_json and spec_summary separately
	diagramVersionIDUsed, specSummary, diagramJSON, err := s.repo.ResolveDiagramContext(ctx, userID, publicID, threadID)
	if err != nil {
		return nil, fmt.Errorf("resolve diagram context: %w", err)
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
	llmReq := chat.ChatRequest{
		Message:     req.Message,
		History:     history,
		Mode:        req.Mode,
		Detail:      req.Detail,
		SpecSummary: specSummary,
		Attachments: attachments,
	}

	// Only include diagram_json if it's not empty and different from spec_summary
	if len(diagramJSON) > 0 && string(diagramJSON) != "{}" && string(diagramJSON) != string(specSummary) {
		llmReq.DiagramJSON = diagramJSON
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
