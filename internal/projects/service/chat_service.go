package service

import (
	"context"
	"fmt"
	"strings"

	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/repository"
)

// ChatService handles chat-related business logic
type ChatService struct {
	repo *repository.ChatRepository
	uigp *dipllm.UIGPClient
}

// NewChatService creates a new chat service
func NewChatService(repo *repository.ChatRepository, uigp *dipllm.UIGPClient) *ChatService {
	return &ChatService{
		repo: repo,
		uigp: uigp,
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

// PostMessage posts a message to a thread and gets an LLM response
func (s *ChatService) PostMessage(ctx context.Context, userID, publicID, threadID string, req PostMessageRequest) (*PostMessageResponse, error) {
	// Resolve diagram context
	diagramVersionIDUsed, spec, err := s.repo.ResolveDiagramContext(ctx, userID, publicID, threadID)
	if err != nil {
		return nil, fmt.Errorf("resolve diagram context: %w", err)
	}

	// Get chat history
	roles, contents, err := s.repo.ListHistoryForUIGP(ctx, userID, publicID, threadID, 20)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}

	// Reverse to chronological order
	history := make([]dipllm.ChatMessage, 0, len(roles))
	for i := len(roles) - 1; i >= 0; i-- {
		history = append(history, dipllm.ChatMessage{
			Role:    roles[i],
			Content: contents[i],
		})
	}

	// Call UIGP client
	uigpResp, err := s.uigp.Chat(ctx, dipllm.ChatRequest{
		SpecSummary: spec,
		History:     history,
		Message:     req.Message,
		Mode:        req.Mode,
	})
	if err != nil {
		return nil, fmt.Errorf("uigp chat: %w", err)
	}

	// Map attachments
	atts := make([]repository.InsertAttachment, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		if strings.TrimSpace(a.ObjectKey) == "" {
			continue
		}
		atts = append(atts, repository.InsertAttachment{
			ObjectKey: strings.TrimSpace(a.ObjectKey),
			MimeType:  a.MimeType,
			FileName:  a.FileName,
			FileSize:  a.FileSize,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	// Insert turn into database
	source := uigpResp.Source
	uMsg, aMsg, err := s.repo.InsertTurn(
		ctx,
		userID, publicID, threadID,
		req.Message,
		uigpResp.Answer,
		&source,
		uigpResp.Refs,
		diagramVersionIDUsed,
		atts,
	)
	if err != nil {
		return nil, fmt.Errorf("insert turn: %w", err)
	}

	return &PostMessageResponse{
		Answer:               uigpResp.Answer,
		Source:               uigpResp.Source,
		Refs:                 uigpResp.Refs,
		DiagramVersionIDUsed: diagramVersionIDUsed,
		UserMessage:          uMsg,
		AssistantMessage:     aMsg,
	}, nil
}

// ListMessages lists messages in a thread
func (s *ChatService) ListMessages(ctx context.Context, userID, publicID, threadID string, limit int) ([]domain.Message, error) {
	return s.repo.ListMessages(ctx, userID, publicID, threadID, limit)
}
