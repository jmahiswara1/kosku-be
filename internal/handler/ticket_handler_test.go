// Package handler_test contains unit tests for the ticket HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database or storage.
// A mock TicketServicer is injected via the interface.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/handler"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/service"
)

// ---------------------------------------------------------------------------
// Mock service
// ---------------------------------------------------------------------------

// stubTicketService is a test double for handler.TicketServicer.
type stubTicketService struct {
	createTicketFn func(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error)
	listTicketsFn  func(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, priority string, page, perPage int) ([]dto.TicketResponse, int64, error)
	getTicketFn    func(ctx context.Context, callerID uuid.UUID, callerRole string, ticketID uuid.UUID) (dto.TicketResponse, error)
	updateTicketFn func(ctx context.Context, ownerID uuid.UUID, ticketID uuid.UUID, req dto.UpdateTicketRequest) (dto.TicketResponse, error)
}

func (m *stubTicketService) CreateTicket(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error) {
	return m.createTicketFn(ctx, tenantID, req, photos, photoContentTypes)
}

func (m *stubTicketService) ListTickets(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, priority string, page, perPage int) ([]dto.TicketResponse, int64, error) {
	return m.listTicketsFn(ctx, ownerID, propertyID, status, priority, page, perPage)
}

func (m *stubTicketService) GetTicket(ctx context.Context, callerID uuid.UUID, callerRole string, ticketID uuid.UUID) (dto.TicketResponse, error) {
	return m.getTicketFn(ctx, callerID, callerRole, ticketID)
}

func (m *stubTicketService) UpdateTicket(ctx context.Context, ownerID uuid.UUID, ticketID uuid.UUID, req dto.UpdateTicketRequest) (dto.TicketResponse, error) {
	return m.updateTicketFn(ctx, ownerID, ticketID, req)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTicketHandlerRouter builds a minimal Gin router that injects userID into
// the context (simulating what the Auth middleware does) and registers the
// ticket handler routes.
func newTicketHandlerRouter(svc handler.TicketServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewTicketHandlerWithService(svc)
	r.POST("/v1/tickets", h.CreateTicket)
	r.GET("/v1/tickets", h.ListTickets)
	r.GET("/v1/tickets/:id", h.GetTicket)
	r.PUT("/v1/tickets/:id", h.UpdateTicket)

	return r
}

// buildTicketMultipartRequest creates a multipart form request for ticket creation.
// It adds the given text fields and optional file parts under the "photos" field.
func buildTicketMultipartRequest(t *testing.T, path string, fields map[string]string, files []struct {
	data        []byte
	contentType string
}) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Write text fields.
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("failed to write field %s: %v", k, err)
		}
	}

	// Write file parts.
	for i, f := range files {
		partHeader := make(map[string][]string)
		partHeader["Content-Disposition"] = []string{`form-data; name="photos"; filename="photo.jpg"`}
		partHeader["Content-Type"] = []string{f.contentType}
		_ = i

		part, err := w.CreatePart(partHeader)
		if err != nil {
			t.Fatalf("failed to create multipart part: %v", err)
		}
		if _, err := part.Write(f.data); err != nil {
			t.Fatalf("failed to write file data: %v", err)
		}
	}
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// decodeTicketBody unmarshals the response body into a map for assertion.
func decodeTicketBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// makeTicketImageData creates a byte slice of the given size filled with dummy data.
func makeTicketImageData(size int) []byte {
	return bytes.Repeat([]byte("x"), size)
}

// ---------------------------------------------------------------------------
// CreateTicket tests — attachment count validation
// ---------------------------------------------------------------------------

// TestCreateTicket_Returns400WhenMoreThan3Photos verifies that
// POST /v1/tickets returns HTTP 400 when more than 3 photo attachments are submitted.
//
// Requirements: 4.1
func TestCreateTicket_Returns400WhenMoreThan3Photos(t *testing.T) {
	tenantID := uuid.New()

	svc := &stubTicketService{
		createTicketFn: func(_ context.Context, _ uuid.UUID, _ dto.CreateTicketRequest, _ [][]byte, _ []string) (dto.TicketResponse, error) {
			// The handler should reject before calling the service when > 3 files are detected.
			// If the service is called, it should also return ErrTooManyAttachments.
			return dto.TicketResponse{}, service.ErrTooManyAttachments
		},
	}

	r := newTicketHandlerRouter(svc, tenantID.String())

	// Build a request with 4 photo attachments (exceeds the limit of 3).
	files := []struct {
		data        []byte
		contentType string
	}{
		{makeTicketImageData(1024), "image/jpeg"},
		{makeTicketImageData(1024), "image/jpeg"},
		{makeTicketImageData(1024), "image/jpeg"},
		{makeTicketImageData(1024), "image/jpeg"}, // 4th file — over the limit
	}

	req := buildTicketMultipartRequest(t, "/v1/tickets", map[string]string{
		"title":       "Broken AC",
		"description": "The air conditioner in my room is not working.",
	}, files)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTicketBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	code, _ := errObj["code"].(string)
	if code != "TOO_MANY_ATTACHMENTS" {
		t.Errorf("expected error code TOO_MANY_ATTACHMENTS, got %v", errObj["code"])
	}
}

// TestCreateTicket_Returns400WhenPhotoExceeds5MB verifies that
// POST /v1/tickets returns HTTP 400 when a photo attachment exceeds 5MB.
//
// Requirements: 4.1
func TestCreateTicket_Returns400WhenPhotoExceeds5MB(t *testing.T) {
	tenantID := uuid.New()

	svc := &stubTicketService{
		createTicketFn: func(_ context.Context, _ uuid.UUID, _ dto.CreateTicketRequest, _ [][]byte, _ []string) (dto.TicketResponse, error) {
			return dto.TicketResponse{}, service.ErrFileTooLarge
		},
	}

	r := newTicketHandlerRouter(svc, tenantID.String())

	// Build a request with a single photo that is 6MB (exceeds the 5MB limit).
	sixMB := 6 * 1024 * 1024
	files := []struct {
		data        []byte
		contentType string
	}{
		{makeTicketImageData(sixMB), "image/jpeg"},
	}

	req := buildTicketMultipartRequest(t, "/v1/tickets", map[string]string{
		"title":       "Leaking pipe",
		"description": "There is a water leak under the sink.",
	}, files)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTicketBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	code, _ := errObj["code"].(string)
	if code != "FILE_TOO_LARGE" {
		t.Errorf("expected error code FILE_TOO_LARGE, got %v", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// CreateTicket tests — success case
// ---------------------------------------------------------------------------

// TestCreateTicket_Returns201WithValidPayload verifies that
// POST /v1/tickets returns HTTP 201 when a valid request with up to 3 photos is submitted.
//
// Requirements: 4.1
func TestCreateTicket_Returns201WithValidPayload(t *testing.T) {
	tenantID := uuid.New()
	ticketID := uuid.New()

	expectedTicket := dto.TicketResponse{
		ID:          ticketID.String(),
		TenantID:    tenantID.String(),
		Title:       "Broken AC",
		Description: "The air conditioner in my room is not working.",
		Priority:    "medium",
		Status:      "open",
	}

	svc := &stubTicketService{
		createTicketFn: func(_ context.Context, tID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, _ []string) (dto.TicketResponse, error) {
			if tID != tenantID {
				t.Errorf("expected tenantID %s, got %s", tenantID, tID)
			}
			if req.Title != "Broken AC" {
				t.Errorf("expected title 'Broken AC', got %s", req.Title)
			}
			if len(photos) != 1 {
				t.Errorf("expected 1 photo, got %d", len(photos))
			}
			return expectedTicket, nil
		},
	}

	r := newTicketHandlerRouter(svc, tenantID.String())

	files := []struct {
		data        []byte
		contentType string
	}{
		{makeTicketImageData(1024), "image/jpeg"},
	}

	req := buildTicketMultipartRequest(t, "/v1/tickets", map[string]string{
		"title":       "Broken AC",
		"description": "The air conditioner in my room is not working.",
	}, files)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTicketBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["status"] != "open" {
		t.Errorf("expected status=open, got %v", data["status"])
	}
}

// ---------------------------------------------------------------------------
// CreateTicket tests — validation
// ---------------------------------------------------------------------------

// TestCreateTicket_Returns400WhenTitleMissing verifies that
// POST /v1/tickets returns HTTP 400 when the title field is absent.
//
// Requirements: 4.1
func TestCreateTicket_Returns400WhenTitleMissing(t *testing.T) {
	tenantID := uuid.New()

	svc := &stubTicketService{
		createTicketFn: func(_ context.Context, _ uuid.UUID, _ dto.CreateTicketRequest, _ [][]byte, _ []string) (dto.TicketResponse, error) {
			t.Error("service.CreateTicket should not be called when validation fails")
			return dto.TicketResponse{}, nil
		},
	}

	r := newTicketHandlerRouter(svc, tenantID.String())

	// Missing "title" field.
	req := buildTicketMultipartRequest(t, "/v1/tickets", map[string]string{
		"description": "Some description without a title.",
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTicketBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}
