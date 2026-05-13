// Package handler_test contains unit tests for the property HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database.
// A mock PropertyServicer is injected via the interface.
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
)

// ---------------------------------------------------------------------------
// Mock service
// ---------------------------------------------------------------------------

// mockPropertyService is a test double for handler.PropertyServicer.
type mockPropertyService struct {
	listFn    func(ctx context.Context, ownerID uuid.UUID) ([]dto.PropertyResponse, error)
	createFn  func(ctx context.Context, ownerID uuid.UUID, req dto.CreatePropertyRequest) (dto.PropertyResponse, error)
	getFn     func(ctx context.Context, ownerID, propertyID uuid.UUID) (dto.PropertyResponse, error)
	updateFn  func(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (dto.PropertyResponse, error)
	archiveFn func(ctx context.Context, ownerID, propertyID uuid.UUID) error
}

func (m *mockPropertyService) ListProperties(ctx context.Context, ownerID uuid.UUID) ([]dto.PropertyResponse, error) {
	return m.listFn(ctx, ownerID)
}

func (m *mockPropertyService) CreateProperty(ctx context.Context, ownerID uuid.UUID, req dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
	return m.createFn(ctx, ownerID, req)
}

func (m *mockPropertyService) GetProperty(ctx context.Context, ownerID, propertyID uuid.UUID) (dto.PropertyResponse, error) {
	return m.getFn(ctx, ownerID, propertyID)
}

func (m *mockPropertyService) UpdateProperty(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (dto.PropertyResponse, error) {
	return m.updateFn(ctx, ownerID, propertyID, req)
}

func (m *mockPropertyService) ArchiveProperty(ctx context.Context, ownerID, propertyID uuid.UUID) error {
	return m.archiveFn(ctx, ownerID, propertyID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newPropertyRouter builds a minimal Gin router that injects userID into the
// context (simulating what the Auth middleware does) and registers the property handler.
func newPropertyRouter(svc handler.PropertyServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewPropertyHandlerWithService(svc)
	r.POST("/v1/properties", h.CreateProperty)
	r.GET("/v1/properties", h.ListProperties)
	r.GET("/v1/properties/:id", h.GetProperty)
	r.PUT("/v1/properties/:id", h.UpdateProperty)
	r.DELETE("/v1/properties/:id", h.DeleteProperty)

	return r
}

// postPropertyJSON performs a POST request with a JSON body and returns the recorder.
func postPropertyJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
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

// decodePropertyBody unmarshals the response body into a map for assertion.
func decodePropertyBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// CreateProperty tests
// ---------------------------------------------------------------------------

// TestCreateProperty_Returns400WhenNameMissing verifies that POST /v1/properties
// returns HTTP 400 when the required "name" field is absent.
//
// Requirements: 2.1
func TestCreateProperty_Returns400WhenNameMissing(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockPropertyService{
		createFn: func(_ context.Context, _ uuid.UUID, _ dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			t.Error("service.CreateProperty should not be called when validation fails")
			return dto.PropertyResponse{}, nil
		},
	}

	r := newPropertyRouter(svc, ownerID.String())
	// Send body without "name".
	w := postPropertyJSON(r, "/v1/properties", map[string]string{
		"address": "Jl. Sudirman No. 1",
		"phone":   "08123456789",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePropertyBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateProperty_Returns400WhenAddressMissing verifies that POST /v1/properties
// returns HTTP 400 when the required "address" field is absent.
//
// Requirements: 2.1
func TestCreateProperty_Returns400WhenAddressMissing(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockPropertyService{
		createFn: func(_ context.Context, _ uuid.UUID, _ dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			t.Error("service.CreateProperty should not be called when validation fails")
			return dto.PropertyResponse{}, nil
		},
	}

	r := newPropertyRouter(svc, ownerID.String())
	// Send body without "address".
	w := postPropertyJSON(r, "/v1/properties", map[string]string{
		"name":  "Kos Melati",
		"phone": "08123456789",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePropertyBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateProperty_Returns400WhenPhoneMissing verifies that POST /v1/properties
// returns HTTP 400 when the required "phone" field is absent.
//
// Requirements: 2.1
func TestCreateProperty_Returns400WhenPhoneMissing(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockPropertyService{
		createFn: func(_ context.Context, _ uuid.UUID, _ dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			t.Error("service.CreateProperty should not be called when validation fails")
			return dto.PropertyResponse{}, nil
		},
	}

	r := newPropertyRouter(svc, ownerID.String())
	// Send body without "phone".
	w := postPropertyJSON(r, "/v1/properties", map[string]string{
		"name":    "Kos Melati",
		"address": "Jl. Sudirman No. 1",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePropertyBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateProperty_Returns400WhenAllRequiredFieldsMissing verifies that
// POST /v1/properties returns HTTP 400 when all required fields are absent.
//
// Requirements: 2.1
func TestCreateProperty_Returns400WhenAllRequiredFieldsMissing(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockPropertyService{
		createFn: func(_ context.Context, _ uuid.UUID, _ dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			t.Error("service.CreateProperty should not be called when validation fails")
			return dto.PropertyResponse{}, nil
		},
	}

	r := newPropertyRouter(svc, ownerID.String())
	// Send empty body.
	w := postPropertyJSON(r, "/v1/properties", map[string]string{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePropertyBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCreateProperty_Returns201WhenAllRequiredFieldsPresent verifies that
// POST /v1/properties returns HTTP 201 when all required fields are provided.
//
// Requirements: 2.1
func TestCreateProperty_Returns201WhenAllRequiredFieldsPresent(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()
	expectedProperty := dto.PropertyResponse{
		ID:      propertyID.String(),
		OwnerID: ownerID.String(),
		Name:    "Kos Melati",
		Address: "Jl. Sudirman No. 1",
		Phone:   "08123456789",
	}

	svc := &mockPropertyService{
		createFn: func(_ context.Context, id uuid.UUID, req dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			if id != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, id)
			}
			if req.Name != "Kos Melati" {
				t.Errorf("expected name=Kos Melati, got %s", req.Name)
			}
			return expectedProperty, nil
		},
	}

	r := newPropertyRouter(svc, ownerID.String())
	w := postPropertyJSON(r, "/v1/properties", map[string]string{
		"name":    "Kos Melati",
		"address": "Jl. Sudirman No. 1",
		"phone":   "08123456789",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodePropertyBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["name"] != "Kos Melati" {
		t.Errorf("expected name=Kos Melati, got %v", data["name"])
	}
}

// TestCreateProperty_Returns401WhenUserIDInvalid verifies that POST /v1/properties
// returns HTTP 401 when the user ID in the context is not a valid UUID.
//
// Requirements: 2.1
func TestCreateProperty_Returns401WhenUserIDInvalid(t *testing.T) {
	svc := &mockPropertyService{
		createFn: func(_ context.Context, _ uuid.UUID, _ dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
			t.Error("service.CreateProperty should not be called with invalid user ID")
			return dto.PropertyResponse{}, nil
		},
	}

	r := newPropertyRouter(svc, "not-a-uuid")
	w := postPropertyJSON(r, "/v1/properties", map[string]string{
		"name":    "Kos Melati",
		"address": "Jl. Sudirman No. 1",
		"phone":   "08123456789",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}
