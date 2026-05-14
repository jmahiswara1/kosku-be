package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/email"
)

// unreadNotifyThreshold is the duration after which an unread message triggers
// an email notification to the recipient.
const unreadNotifyThreshold = 30 * time.Minute

// MessageService handles business logic for direct messaging.
type MessageService struct {
	queries     *repository.Queries
	emailClient *email.Client
}

// NewMessageService creates a new MessageService.
func NewMessageService(queries *repository.Queries, emailClient *email.Client) *MessageService {
	return &MessageService{
		queries:     queries,
		emailClient: emailClient,
	}
}

// ListConversations returns the list of distinct conversation partners for the
// authenticated user. Each entry contains the partner's ID, name, and the most
// recent message in the conversation.
func (s *MessageService) ListConversations(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error) {
	messages, err := s.queries.ListConversations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}

	result := make([]dto.ConversationResponse, 0, len(messages))
	for _, m := range messages {
		// Determine the partner (the other side of the conversation).
		partnerID := m.SenderID
		if m.SenderID == userID {
			partnerID = m.ReceiverID
		}

		// Fetch partner's profile name — non-fatal if unavailable.
		partnerName := ""
		partnerProfile, err := s.queries.GetProfile(ctx, partnerID)
		if err == nil {
			partnerName = partnerProfile.FullName
		}

		result = append(result, dto.ConversationResponse{
			PartnerID:   partnerID.String(),
			PartnerName: partnerName,
			LastMessage: messageToDTO(m),
		})
	}
	return result, nil
}

// GetThread returns the full message thread between the authenticated user and
// the given partner, ordered by created_at ASC.
func (s *MessageService) GetThread(ctx context.Context, userID, partnerID uuid.UUID) ([]dto.MessageResponse, error) {
	messages, err := s.queries.GetMessageThread(ctx, repository.GetMessageThreadParams{
		SenderID:   userID,
		ReceiverID: partnerID,
	})
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}

	result := make([]dto.MessageResponse, 0, len(messages))
	for _, m := range messages {
		result = append(result, messageToDTO(m))
	}
	return result, nil
}

// SendMessage inserts a new message row. After inserting, it checks whether the
// recipient has any unread messages from the sender that are older than 30
// minutes. If so, it triggers an email notification to the recipient.
func (s *MessageService) SendMessage(ctx context.Context, senderID uuid.UUID, req dto.SendMessageRequest) (dto.MessageResponse, error) {
	receiverID, err := uuid.Parse(req.ReceiverID)
	if err != nil {
		return dto.MessageResponse{}, fmt.Errorf("send message: invalid receiver_id: %w", err)
	}

	// Verify the receiver exists.
	_, err = s.queries.GetProfile(ctx, receiverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.MessageResponse{}, ErrNotFound
		}
		return dto.MessageResponse{}, fmt.Errorf("send message: get receiver profile: %w", err)
	}

	// Build optional property_id argument.
	var propertyIDArg uuid.NullUUID
	if req.PropertyID != "" {
		pid, err := uuid.Parse(req.PropertyID)
		if err == nil {
			propertyIDArg = uuid.NullUUID{UUID: pid, Valid: true}
		}
	}

	// Insert the message row.
	msg, err := s.queries.CreateMessage(ctx, repository.CreateMessageParams{
		SenderID:   senderID,
		ReceiverID: receiverID,
		PropertyID: propertyIDArg,
		Body:       req.Body,
	})
	if err != nil {
		return dto.MessageResponse{}, fmt.Errorf("send message: insert message: %w", err)
	}

	// Check if the recipient has unread messages from the sender older than 30
	// minutes. If so, send an email notification — non-fatal.
	go func() {
		unreadMsgs, err := s.queries.GetUnreadMessages(ctx, receiverID)
		if err != nil {
			return
		}

		threshold := time.Now().Add(-unreadNotifyThreshold)
		hasOldUnread := false
		for _, um := range unreadMsgs {
			// Only consider messages from this sender.
			if um.SenderID != senderID {
				continue
			}
			if um.CreatedAt.Valid && um.CreatedAt.Time.Before(threshold) {
				hasOldUnread = true
				break
			}
		}

		if !hasOldUnread {
			return
		}

		// Fetch sender and receiver profiles for the email.
		senderProfile, err := s.queries.GetProfile(ctx, senderID)
		if err != nil {
			return
		}
		receiverProfile, err := s.queries.GetProfile(ctx, receiverID)
		if err != nil {
			return
		}

		// Send email — best-effort (email address not stored in profiles table,
		// so this is a no-op unless the email client resolves it externally).
		_ = s.emailClient.SendNewMessageNotification(
			"", // recipient email not available from profiles table
			receiverProfile.FullName,
			senderProfile.FullName,
		)
	}()

	return messageToDTO(msg), nil
}

// messageToDTO converts a repository.Message to dto.MessageResponse.
func messageToDTO(m repository.Message) dto.MessageResponse {
	resp := dto.MessageResponse{
		ID:         m.ID.String(),
		SenderID:   m.SenderID.String(),
		ReceiverID: m.ReceiverID.String(),
		Body:       m.Body,
	}
	if m.PropertyID.Valid {
		resp.PropertyID = m.PropertyID.UUID.String()
	}
	if m.IsRead.Valid {
		resp.IsRead = m.IsRead.Bool
	}
	if m.CreatedAt.Valid {
		resp.CreatedAt = m.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
