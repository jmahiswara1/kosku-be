// Package handler_test contains unit tests for the messaging HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database.
// A mock MessageServicer is injected via the interface.
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/handler"
	"github.com/kosku/backend/internal/middleware"
)

// ---------------------------------------------------------------------------
// Mock service
// ---------------------------------------------------------------------------

// stubMessageService is a test double for handler.MessageServicer.
type stubMessageService struct {
	listConversationsFn func(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error)
	getThreadFn         func(ctx context.Context, userID, partnerID uuid.UUID) ([]dto.MessageResponse, error)
	sendMessageFn       func(ctx context.Context, senderID uuid.UUID, req dto.SendMessageRequest) (dto.MessageResponse, error)
}

func (m *stubMessageService) ListConversations(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error) {
	return m.listConversationsFn(ctx, userID)
}

func (m *stubMessageService) GetThread(ctx context.Context, userID, partnerID uuid.UUID) ([]dto.MessageResponse, error) {
	return m.getThreadFn(ctx, userID, partnerID)
}

func (m *stubMessageService) SendMessage(ctx context.Context, senderID uuid.UUID, req dto.SendMessageRequest) (dto.MessageResponse, error) {
	return m.sendMessageFn(ctx, senderID, req)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMessageHandlerRouter builds a minimal Gin router that injects userID into
// the context (simulating what the Auth middleware does) and registers the
// message handler routes.
func newMessageHandlerRouter(svc handler.MessageServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewMessageHandlerWithService(svc)
	r.GET("/v1/messages", h.ListConversations)
	r.GET("/v1/messages/:userId", h.GetThread)
	r.POST("/v1/messages", h.SendMessage)

	return r
}

// decodeMessageBody unmarshals the response body into a map for assertion.
func decodeMessageBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// GetThread tests — chronological ordering
// ---------------------------------------------------------------------------

// TestGetThread_ReturnsMessagesInChronologicalOrder verifies that
// GET /v1/messages/:userId returns messages ordered by created_at ASC.
//
// Requirements: 9.2
func TestGetThread_ReturnsMessagesInChronologicalOrder(t *testing.T) {
	userID := uuid.New()
	partnerID := uuid.New()

	// Build messages with timestamps in ascending order (oldest first).
	now := time.Now().UTC()
	messages := []dto.MessageResponse{
		{
			ID:         uuid.New().String(),
			SenderID:   userID.String(),
			ReceiverID: partnerID.String(),
			Body:       "Hello!",
			CreatedAt:  now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:         uuid.New().String(),
			SenderID:   partnerID.String(),
			ReceiverID: userID.String(),
			Body:       "Hi there!",
			CreatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:         uuid.New().String(),
			SenderID:   userID.String(),
			ReceiverID: partnerID.String(),
			Body:       "How are you?",
			CreatedAt:  now.Format(time.RFC3339),
		},
	}

	svc := &stubMessageService{
		getThreadFn: func(_ context.Context, uID, pID uuid.UUID) ([]dto.MessageResponse, error) {
			if uID != userID {
				t.Errorf("expected userID %s, got %s", userID, uID)
			}
			if pID != partnerID {
				t.Errorf("expected partnerID %s, got %s", partnerID, pID)
			}
			// Return messages already ordered by created_at ASC (as the service/DB guarantees).
			return messages, nil
		},
	}

	r := newMessageHandlerRouter(svc, userID.String())
	req := httptest.NewRequest(http.MethodGet, "/v1/messages/"+partnerID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeMessageBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", body["data"])
	}
	if len(data) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(data))
	}

	// Verify messages are in chronological order (created_at ASC).
	var prevTime time.Time
	for i, item := range data {
		msg, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected message[%d] to be an object, got %T", i, item)
		}

		createdAtStr, ok := msg["created_at"].(string)
		if !ok {
			t.Fatalf("expected message[%d].created_at to be a string, got %T", i, msg["created_at"])
		}

		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			t.Fatalf("failed to parse message[%d].created_at %q: %v", i, createdAtStr, err)
		}

		if i > 0 && createdAt.Before(prevTime) {
			t.Errorf("message[%d] created_at %v is before message[%d] created_at %v — not in chronological order",
				i, createdAt, i-1, prevTime)
		}
		prevTime = createdAt
	}

	// Verify the first message is the oldest and the last is the newest.
	firstMsg := data[0].(map[string]interface{})
	lastMsg := data[len(data)-1].(map[string]interface{})

	if firstMsg["body"] != "Hello!" {
		t.Errorf("expected first message body 'Hello!', got %v", firstMsg["body"])
	}
	if lastMsg["body"] != "How are you?" {
		t.Errorf("expected last message body 'How are you?', got %v", lastMsg["body"])
	}
}

// TestGetThread_ReturnsEmptyArrayWhenNoMessages verifies that
// GET /v1/messages/:userId returns an empty array when there are no messages.
//
// Requirements: 9.2
func TestGetThread_ReturnsEmptyArrayWhenNoMessages(t *testing.T) {
	userID := uuid.New()
	partnerID := uuid.New()

	svc := &stubMessageService{
		getThreadFn: func(_ context.Context, _, _ uuid.UUID) ([]dto.MessageResponse, error) {
			return []dto.MessageResponse{}, nil
		},
	}

	r := newMessageHandlerRouter(svc, userID.String())
	req := httptest.NewRequest(http.MethodGet, "/v1/messages/"+partnerID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeMessageBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", body["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected 0 messages, got %d", len(data))
	}

	meta, ok := body["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected meta to be an object, got %T", body["meta"])
	}
	if meta["total"] != float64(0) {
		t.Errorf("expected meta.total=0, got %v", meta["total"])
	}
}

// TestGetThread_Returns400WhenPartnerIDInvalid verifies that
// GET /v1/messages/:userId returns HTTP 400 when the partner ID is not a valid UUID.
//
// Requirements: 9.2
func TestGetThread_Returns400WhenPartnerIDInvalid(t *testing.T) {
	userID := uuid.New()

	svc := &stubMessageService{
		getThreadFn: func(_ context.Context, _, _ uuid.UUID) ([]dto.MessageResponse, error) {
			t.Error("service.GetThread should not be called with invalid partner ID")
			return nil, nil
		},
	}

	r := newMessageHandlerRouter(svc, userID.String())
	req := httptest.NewRequest(http.MethodGet, "/v1/messages/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeMessageBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestGetThread_ReturnsSingleMessageInOrder verifies that a thread with a single
// message is returned correctly (edge case for ordering logic).
//
// Requirements: 9.2
func TestGetThread_ReturnsSingleMessageInOrder(t *testing.T) {
	userID := uuid.New()
	partnerID := uuid.New()
	msgID := uuid.New()

	singleMessage := []dto.MessageResponse{
		{
			ID:         msgID.String(),
			SenderID:   userID.String(),
			ReceiverID: partnerID.String(),
			Body:       "Just one message",
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		},
	}

	svc := &stubMessageService{
		getThreadFn: func(_ context.Context, _, _ uuid.UUID) ([]dto.MessageResponse, error) {
			return singleMessage, nil
		},
	}

	r := newMessageHandlerRouter(svc, userID.String())
	req := httptest.NewRequest(http.MethodGet, "/v1/messages/"+partnerID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeMessageBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", body["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 message, got %d", len(data))
	}

	msg := data[0].(map[string]interface{})
	if msg["id"] != msgID.String() {
		t.Errorf("expected message id %s, got %v", msgID.String(), msg["id"])
	}
	if msg["body"] != "Just one message" {
		t.Errorf("expected body 'Just one message', got %v", msg["body"])
	}
}
