// Package handler_test contains unit tests for the room HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database.
// A mock RoomServicer is injected via the interface.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// stubRoomService is a test double for handler.RoomServicer used in unit tests.
// (The property-based test file defines its own mockRoomService; this avoids conflicts.)
type stubRoomService struct {
	listFn           func(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error)
	createFn         func(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error)
	getFn            func(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error)
	updateFn         func(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error)
	archiveFn        func(ctx context.Context, ownerID, roomID uuid.UUID) error
	updateLayoutFn   func(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error
	getRoomHistoryFn func(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error)
}

func (m *stubRoomService) ListRooms(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error) {
	return m.listFn(ctx, ownerID, propertyID)
}

func (m *stubRoomService) CreateRoom(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error) {
	return m.createFn(ctx, ownerID, propertyID, req)
}

func (m *stubRoomService) GetRoom(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error) {
	return m.getFn(ctx, ownerID, roomID)
}

func (m *stubRoomService) UpdateRoom(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error) {
	return m.updateFn(ctx, ownerID, roomID, req)
}

func (m *stubRoomService) ArchiveRoom(ctx context.Context, ownerID, roomID uuid.UUID) error {
	return m.archiveFn(ctx, ownerID, roomID)
}

func (m *stubRoomService) UpdateLayout(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error {
	return m.updateLayoutFn(ctx, ownerID, propertyID, req)
}

func (m *stubRoomService) GetRoomHistory(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error) {
	return m.getRoomHistoryFn(ctx, ownerID, roomID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newRoomHandlerRouter builds a minimal Gin router that injects userID into the
// context (simulating what the Auth middleware does) and registers the room handler.
func newRoomHandlerRouter(svc handler.RoomServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewRoomHandlerWithService(svc)
	r.GET("/v1/properties/:id/rooms", h.ListRooms)
	r.POST("/v1/properties/:id/rooms", h.CreateRoom)
	r.GET("/v1/rooms/:id", h.GetRoom)
	r.PUT("/v1/rooms/:id", h.UpdateRoom)
	r.DELETE("/v1/rooms/:id", h.DeleteRoom)
	r.PUT("/v1/properties/:id/layout", h.UpdateLayout)
	r.GET("/v1/rooms/:id/history", h.GetRoomHistory)

	return r
}

// postRoomJSON performs a POST request with a JSON body and returns the recorder.
func postRoomJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeRoomBody unmarshals the response body into a map for assertion.
func decodeRoomBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// CreateRoom tests
// ---------------------------------------------------------------------------

// TestCreateRoom_Returns409WhenRoomNumberAlreadyExists verifies that
// POST /v1/properties/:id/rooms returns HTTP 409 when the room number already
// exists in the same property.
//
// Requirements: 2.2
func TestCreateRoom_Returns409WhenRoomNumberAlreadyExists(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	svc := &stubRoomService{
		createFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error) {
			// Simulate the service returning a duplicate room number error.
			return dto.RoomResponse{}, service.ErrDuplicateRoomNumber
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	w := postRoomJSON(r, "/v1/properties/"+propertyID.String()+"/rooms", map[string]interface{}{
		"number":         "101",
		"room_type_name": "Standard",
		"monthly_price":  "500000",
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeRoomBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "DUPLICATE_ROOM_NUMBER" {
		t.Errorf("expected error code DUPLICATE_ROOM_NUMBER, got %v", errObj["code"])
	}
}

// TestCreateRoom_Returns400WhenNumberMissing verifies that
// POST /v1/properties/:id/rooms returns HTTP 400 when the required "number" field is absent.
//
// Requirements: 2.2
func TestCreateRoom_Returns400WhenNumberMissing(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	svc := &stubRoomService{
		createFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CreateRoomRequest) (dto.RoomResponse, error) {
			t.Error("service.CreateRoom should not be called when validation fails")
			return dto.RoomResponse{}, nil
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	// Send body without "number".
	w := postRoomJSON(r, "/v1/properties/"+propertyID.String()+"/rooms", map[string]interface{}{
		"room_type_name": "Standard",
		"monthly_price":  "500000",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeRoomBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateRoom_Returns400WhenRoomTypeNameMissing verifies that
// POST /v1/properties/:id/rooms returns HTTP 400 when the required "room_type_name" field is absent.
//
// Requirements: 2.2
func TestCreateRoom_Returns400WhenRoomTypeNameMissing(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	svc := &stubRoomService{
		createFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CreateRoomRequest) (dto.RoomResponse, error) {
			t.Error("service.CreateRoom should not be called when validation fails")
			return dto.RoomResponse{}, nil
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	// Send body without "room_type_name".
	w := postRoomJSON(r, "/v1/properties/"+propertyID.String()+"/rooms", map[string]interface{}{
		"number":        "101",
		"monthly_price": "500000",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeRoomBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateRoom_Returns400WhenMonthlyPriceMissing verifies that
// POST /v1/properties/:id/rooms returns HTTP 400 when the required "monthly_price" field is absent.
//
// Requirements: 2.2
func TestCreateRoom_Returns400WhenMonthlyPriceMissing(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	svc := &stubRoomService{
		createFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CreateRoomRequest) (dto.RoomResponse, error) {
			t.Error("service.CreateRoom should not be called when validation fails")
			return dto.RoomResponse{}, nil
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	// Send body without "monthly_price".
	w := postRoomJSON(r, "/v1/properties/"+propertyID.String()+"/rooms", map[string]interface{}{
		"number":         "101",
		"room_type_name": "Standard",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeRoomBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateRoom_Returns201OnSuccess verifies that POST /v1/properties/:id/rooms
// returns HTTP 201 when all required fields are provided and the room number is unique.
//
// Requirements: 2.2
func TestCreateRoom_Returns201OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()
	roomID := uuid.New()
	expectedRoom := dto.RoomResponse{
		ID:         roomID.String(),
		PropertyID: propertyID.String(),
		Number:     "101",
		Status:     "vacant",
	}

	svc := &stubRoomService{
		createFn: func(_ context.Context, oID uuid.UUID, pID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if pID != propertyID {
				t.Errorf("expected propertyID %s, got %s", propertyID, pID)
			}
			if req.Number != "101" {
				t.Errorf("expected number=101, got %s", req.Number)
			}
			return expectedRoom, nil
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	w := postRoomJSON(r, "/v1/properties/"+propertyID.String()+"/rooms", map[string]interface{}{
		"number":         "101",
		"room_type_name": "Standard",
		"monthly_price":  "500000",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeRoomBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["number"] != "101" {
		t.Errorf("expected number=101, got %v", data["number"])
	}
}

// TestCreateRoom_Returns400WhenPropertyIDInvalid verifies that
// POST /v1/properties/:id/rooms returns HTTP 400 when the property ID path param is not a valid UUID.
//
// Requirements: 2.2
func TestCreateRoom_Returns400WhenPropertyIDInvalid(t *testing.T) {
	ownerID := uuid.New()

	svc := &stubRoomService{
		createFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CreateRoomRequest) (dto.RoomResponse, error) {
			t.Error("service.CreateRoom should not be called with invalid property ID")
			return dto.RoomResponse{}, nil
		},
	}

	r := newRoomHandlerRouter(svc, ownerID.String())
	w := postRoomJSON(r, "/v1/properties/not-a-uuid/rooms", map[string]interface{}{
		"number":         "101",
		"room_type_name": "Standard",
		"monthly_price":  "500000",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}
