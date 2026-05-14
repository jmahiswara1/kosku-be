package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
)

// NotificationService handles business logic for notification management.
type NotificationService struct {
	queries *repository.Queries
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(queries *repository.Queries) *NotificationService {
	return &NotificationService{queries: queries}
}

// ListNotifications returns all notifications for the given user, ordered by
// created_at DESC.
func (s *NotificationService) ListNotifications(ctx context.Context, userID uuid.UUID) ([]dto.NotificationResponse, error) {
	rows, err := s.queries.ListNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	result := make([]dto.NotificationResponse, 0, len(rows))
	for _, n := range rows {
		result = append(result, notificationToDTO(n))
	}
	return result, nil
}

// MarkAllRead marks all unread notifications for the user as read.
func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.queries.MarkAllNotificationsRead(ctx, userID); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

// MarkOneRead marks a single notification as read.
// It returns ErrNotFound if the notification does not exist or does not belong
// to the user (the SQL WHERE clause enforces ownership).
func (s *NotificationService) MarkOneRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if err := s.queries.MarkNotificationRead(ctx, repository.MarkNotificationReadParams{
		ID:     notificationID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	return nil
}

// notificationToDTO converts a repository.Notification to dto.NotificationResponse.
func notificationToDTO(n repository.Notification) dto.NotificationResponse {
	resp := dto.NotificationResponse{
		ID:     n.ID.String(),
		UserID: n.UserID.String(),
		Type:   n.Type,
		Title:  n.Title,
	}
	if n.Body.Valid {
		resp.Body = n.Body.String
	}
	if n.EntityID.Valid {
		resp.EntityID = n.EntityID.UUID.String()
	}
	if n.IsRead.Valid {
		resp.IsRead = n.IsRead.Bool
	}
	if n.CreatedAt.Valid {
		resp.CreatedAt = n.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
