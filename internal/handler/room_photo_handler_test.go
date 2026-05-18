// Package handler_test contains unit tests for the room photo HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database or storage.
// A mock RoomPhotoServicer is injected via the interface.
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

// mockRoomPhotoService is a test double for handler.RoomPhotoServicer.
type mockRoomPhotoService struct {
	uploadFn func(ctx context.Context, ownerID, roomID uuid.UUID, fileData []byte, declaredContentType string) (dto.RoomPhotoResponse, error)
	deleteFn func(ctx context.Context, ownerID, roomID, photoID uuid.UUID) error
}

func (m *mockRoomPhotoService) UploadPhoto(ctx context.Context, ownerID, roomID uuid.UUID, fileData []byte, declaredContentType string) (dto.RoomPhotoResponse, error) {
	return m.uploadFn(ctx, ownerID, roomID, fileData, declaredContentType)
}

func (m *mockRoomPhotoService) DeletePhoto(ctx context.Context, ownerID, roomID, photoID uuid.UUID) error {
	return m.deleteFn(ctx, ownerID, roomID, photoID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newRoomPhotoRouter builds a minimal Gin router that injects userID into the
// context (simulating what the Auth middleware does) and registers the room photo handler.
func newRoomPhotoRouter(svc handler.RoomPhotoServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewRoomPhotoHandlerWithService(svc)
	r.POST("/v1/rooms/:id/photos", h.UploadPhoto)
	r.DELETE("/v1/rooms/:id/photos/:photoId", h.DeletePhoto)

	return r
}

// buildMultipartRequest creates an HTTP request with a multipart form containing
// a "photo" file field. The contentType is set on the part header.
func buildMultipartRequest(t *testing.T, path, contentType string, fileData []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Create the file part with the given content type.
	partHeader := make(map[string][]string)
	partHeader["Content-Disposition"] = []string{`form-data; name="photo"; filename="test.jpg"`}
	partHeader["Content-Type"] = []string{contentType}

	part, err := w.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("failed to create multipart part: %v", err)
	}
	if _, err := part.Write(fileData); err != nil {
		t.Fatalf("failed to write file data: %v", err)
	}
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// decodePhotoBody unmarshals the response body into a map for assertion.
func decodePhotoBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// makeImageData creates a byte slice of the given size filled with dummy data.
func makeImageData(size int) []byte {
	return bytes.Repeat([]byte("x"), size)
}

// ---------------------------------------------------------------------------
// UploadPhoto tests — file size validation
// ---------------------------------------------------------------------------

// TestUploadPhoto_Returns400WhenFileTooLarge verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when the uploaded file exceeds 5MB.
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenFileTooLarge(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			// Simulate the service returning a file-too-large error.
			return dto.RoomPhotoResponse{}, service.ErrFileTooLarge
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	// Create a file that is 6MB (exceeds the 5MB limit).
	sixMB := 6 * 1024 * 1024
	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/jpeg", makeImageData(sixMB))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "FILE_TOO_LARGE" {
		t.Errorf("expected error code FILE_TOO_LARGE, got %v", errObj["code"])
	}
}

// TestUploadPhoto_Returns400WhenFileSizeExactly5MBPlusOne verifies that
// a file of exactly 5MB + 1 byte is rejected with HTTP 400.
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenFileSizeExactly5MBPlusOne(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			return dto.RoomPhotoResponse{}, service.ErrFileTooLarge
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	// 5MB + 1 byte.
	overLimit := 5*1024*1024 + 1
	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/jpeg", makeImageData(overLimit))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// ---------------------------------------------------------------------------
// UploadPhoto tests — MIME type validation
// ---------------------------------------------------------------------------

// TestUploadPhoto_Returns400WhenMIMETypeIsNotImage verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when the uploaded file has a
// non-image MIME type (e.g., application/pdf).
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenMIMETypeIsNotImage(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			// Simulate the service returning an invalid file type error.
			return dto.RoomPhotoResponse{}, service.ErrInvalidFileType
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "application/pdf", makeImageData(1024))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "INVALID_FILE_TYPE" {
		t.Errorf("expected error code INVALID_FILE_TYPE, got %v", errObj["code"])
	}
}

// TestUploadPhoto_Returns400WhenMIMETypeIsTextPlain verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when the uploaded file has
// text/plain MIME type.
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenMIMETypeIsTextPlain(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			return dto.RoomPhotoResponse{}, service.ErrInvalidFileType
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "text/plain", []byte("not an image"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestUploadPhoto_Returns400WhenMIMETypeIsGif verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when the uploaded file has
// image/gif MIME type (not in the allowed list of jpeg/png/webp).
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenMIMETypeIsGif(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			return dto.RoomPhotoResponse{}, service.ErrInvalidFileType
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/gif", makeImageData(1024))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// ---------------------------------------------------------------------------
// UploadPhoto tests — success cases
// ---------------------------------------------------------------------------

// TestUploadPhoto_Returns201ForJPEG verifies that POST /v1/rooms/:id/photos
// returns HTTP 201 when a valid JPEG file within the size limit is uploaded.
//
// Requirements: 2.2
func TestUploadPhoto_Returns201ForJPEG(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()
	photoID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, oID uuid.UUID, rID uuid.UUID, _ []byte, contentType string) (dto.RoomPhotoResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if rID != roomID {
				t.Errorf("expected roomID %s, got %s", roomID, rID)
			}
			return dto.RoomPhotoResponse{
				ID:     photoID.String(),
				RoomID: roomID.String(),
				URL:    "https://storage.example.com/room-photos/" + photoID.String() + ".jpg",
			}, nil
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	// 1KB JPEG — well within the 5MB limit.
	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/jpeg", makeImageData(1024))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["id"] != photoID.String() {
		t.Errorf("expected id=%s, got %v", photoID.String(), data["id"])
	}
}

// TestUploadPhoto_Returns201ForPNG verifies that POST /v1/rooms/:id/photos
// returns HTTP 201 when a valid PNG file is uploaded.
//
// Requirements: 2.2
func TestUploadPhoto_Returns201ForPNG(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()
	photoID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			return dto.RoomPhotoResponse{
				ID:     photoID.String(),
				RoomID: roomID.String(),
				URL:    "https://storage.example.com/room-photos/" + photoID.String() + ".png",
			}, nil
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/png", makeImageData(2048))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestUploadPhoto_Returns201ForWebP verifies that POST /v1/rooms/:id/photos
// returns HTTP 201 when a valid WebP file is uploaded.
//
// Requirements: 2.2
func TestUploadPhoto_Returns201ForWebP(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()
	photoID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			return dto.RoomPhotoResponse{
				ID:     photoID.String(),
				RoomID: roomID.String(),
				URL:    "https://storage.example.com/room-photos/" + photoID.String() + ".webp",
			}, nil
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/"+roomID.String()+"/photos", "image/webp", makeImageData(2048))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// UploadPhoto tests — missing file
// ---------------------------------------------------------------------------

// TestUploadPhoto_Returns400WhenNoFileProvided verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when no file is included in the request.
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenNoFileProvided(t *testing.T) {
	ownerID := uuid.New()
	roomID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			t.Error("service.UploadPhoto should not be called when no file is provided")
			return dto.RoomPhotoResponse{}, nil
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	// Send a multipart form without a "photo" field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("other_field", "value")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/rooms/"+roomID.String()+"/photos", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePhotoBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestUploadPhoto_Returns400WhenRoomIDInvalid verifies that
// POST /v1/rooms/:id/photos returns HTTP 400 when the room ID path param is not a valid UUID.
//
// Requirements: 2.2
func TestUploadPhoto_Returns400WhenRoomIDInvalid(t *testing.T) {
	ownerID := uuid.New()

	svc := &mockRoomPhotoService{
		uploadFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string) (dto.RoomPhotoResponse, error) {
			t.Error("service.UploadPhoto should not be called with invalid room ID")
			return dto.RoomPhotoResponse{}, nil
		},
	}

	r := newRoomPhotoRouter(svc, ownerID.String())

	req := buildMultipartRequest(t, "/v1/rooms/not-a-uuid/photos", "image/jpeg", makeImageData(1024))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}
