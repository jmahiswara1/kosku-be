// Package handler_test contains unit tests for the tenant HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database.
// A mock TenantServicer is injected via the interface.
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

// Mock service

// stubTenantService is a test double for handler.TenantServicer used in unit tests.
type stubTenantService struct {
	listFn      func(ctx context.Context, ownerID uuid.UUID, propertyID *uuid.UUID, limit, offset int) ([]dto.TenantResponse, int, error)
	getFn       func(ctx context.Context, ownerID, tenantID uuid.UUID) (dto.TenantResponse, error)
	updateFn    func(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.UpdateTenantRequest) (dto.TenantResponse, error)
	checkinFn   func(ctx context.Context, ownerID uuid.UUID, req dto.CheckinRequest) (dto.CheckinResponse, error)
	checkoutFn  func(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.CheckoutRequest) (dto.ContractResponse, error)
	blacklistFn func(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.BlacklistRequest) (dto.TenantResponse, error)
}

func (m *stubTenantService) ListTenants(ctx context.Context, ownerID uuid.UUID, propertyID *uuid.UUID, limit, offset int) ([]dto.TenantResponse, int, error) {
	return m.listFn(ctx, ownerID, propertyID, limit, offset)
}

func (m *stubTenantService) GetTenant(ctx context.Context, ownerID, tenantID uuid.UUID) (dto.TenantResponse, error) {
	return m.getFn(ctx, ownerID, tenantID)
}

func (m *stubTenantService) UpdateTenant(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.UpdateTenantRequest) (dto.TenantResponse, error) {
	return m.updateFn(ctx, ownerID, tenantID, req)
}

func (m *stubTenantService) Checkin(ctx context.Context, ownerID uuid.UUID, req dto.CheckinRequest) (dto.CheckinResponse, error) {
	return m.checkinFn(ctx, ownerID, req)
}

func (m *stubTenantService) Checkout(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.CheckoutRequest) (dto.ContractResponse, error) {
	return m.checkoutFn(ctx, ownerID, tenantID, req)
}

func (m *stubTenantService) Blacklist(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.BlacklistRequest) (dto.TenantResponse, error) {
	return m.blacklistFn(ctx, ownerID, tenantID, req)
}

// Helpers

// newTenantHandlerRouter builds a minimal Gin router that injects userID into
// the context (simulating what the Auth middleware does) and registers the
// tenant handler routes.
func newTenantHandlerRouter(svc handler.TenantServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewTenantHandlerWithService(svc)
	// Register specific routes BEFORE parameterized routes to avoid conflicts.
	r.POST("/v1/tenants/checkin", h.Checkin)
	r.POST("/v1/tenants/checkout/:id", h.Checkout)
	r.GET("/v1/tenants", h.ListTenants)
	r.GET("/v1/tenants/:id", h.GetTenant)
	r.PUT("/v1/tenants/:id", h.UpdateTenant)
	r.POST("/v1/tenants/:id/blacklist", h.Blacklist)

	return r
}

// postTenantJSON performs a POST request with a JSON body and returns the recorder.
func postTenantJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
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

// decodeTenantBody unmarshals the response body into a map for assertion.
func decodeTenantBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// Checkin tests

// TestCheckin_FailsIfRoomNotVacant verifies that POST /v1/tenants/checkin
// returns HTTP 409 when the room is not vacant.
//
// Requirements: 3.2
func TestCheckin_FailsIfRoomNotVacant(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()
	roomID := uuid.New()
	propertyID := uuid.New()

	svc := &stubTenantService{
		checkinFn: func(_ context.Context, _ uuid.UUID, _ dto.CheckinRequest) (dto.CheckinResponse, error) {
			return dto.CheckinResponse{}, service.ErrRoomNotVacant
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	w := postTenantJSON(r, "/v1/tenants/checkin", map[string]interface{}{
		"tenant_id":     tenantID.String(),
		"room_id":       roomID.String(),
		"property_id":   propertyID.String(),
		"start_date":    "2024-01-01T00:00:00Z",
		"end_date":      "2024-12-31T00:00:00Z",
		"monthly_price": 1500000,
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "ROOM_NOT_VACANT" {
		t.Errorf("expected error code ROOM_NOT_VACANT, got %v", errObj["code"])
	}
}

// TestCheckin_FailsIfTenantBlacklisted verifies that POST /v1/tenants/checkin
// returns HTTP 409 when the tenant is blacklisted.
//
// Requirements: 3.2
func TestCheckin_FailsIfTenantBlacklisted(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()
	roomID := uuid.New()
	propertyID := uuid.New()

	svc := &stubTenantService{
		checkinFn: func(_ context.Context, _ uuid.UUID, _ dto.CheckinRequest) (dto.CheckinResponse, error) {
			return dto.CheckinResponse{}, service.ErrTenantBlacklisted
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	w := postTenantJSON(r, "/v1/tenants/checkin", map[string]interface{}{
		"tenant_id":     tenantID.String(),
		"room_id":       roomID.String(),
		"property_id":   propertyID.String(),
		"start_date":    "2024-01-01T00:00:00Z",
		"end_date":      "2024-12-31T00:00:00Z",
		"monthly_price": 1500000,
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "TENANT_BLACKLISTED" {
		t.Errorf("expected error code TENANT_BLACKLISTED, got %v", errObj["code"])
	}
}

// TestCheckin_Returns400WhenRequiredFieldsMissing verifies that
// POST /v1/tenants/checkin returns HTTP 400 when required fields are absent.
//
// Requirements: 3.2
func TestCheckin_Returns400WhenRequiredFieldsMissing(t *testing.T) {
	ownerID := uuid.New()

	svc := &stubTenantService{
		checkinFn: func(_ context.Context, _ uuid.UUID, _ dto.CheckinRequest) (dto.CheckinResponse, error) {
			t.Error("service.Checkin should not be called when validation fails")
			return dto.CheckinResponse{}, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	// Missing required fields.
	w := postTenantJSON(r, "/v1/tenants/checkin", map[string]interface{}{
		"tenant_id": uuid.New().String(),
		// missing room_id, property_id, start_date, end_date, monthly_price
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestCheckin_Returns201OnSuccess verifies that POST /v1/tenants/checkin
// returns HTTP 201 when all required fields are provided and the room is vacant.
//
// Requirements: 3.2
func TestCheckin_Returns201OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()
	roomID := uuid.New()
	propertyID := uuid.New()
	contractID := uuid.New()

	expectedResponse := dto.CheckinResponse{
		Tenant: dto.TenantResponse{
			ID:       tenantID.String(),
			FullName: "John Doe",
			RoomID:   roomID.String(),
		},
		Contract: dto.ContractResponse{
			ID:           contractID.String(),
			TenantID:     tenantID.String(),
			RoomID:       roomID.String(),
			PropertyID:   propertyID.String(),
			Status:       "active",
			MonthlyPrice: "1500000.00",
		},
	}

	svc := &stubTenantService{
		checkinFn: func(_ context.Context, oID uuid.UUID, req dto.CheckinRequest) (dto.CheckinResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			return expectedResponse, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	w := postTenantJSON(r, "/v1/tenants/checkin", map[string]interface{}{
		"tenant_id":     tenantID.String(),
		"room_id":       roomID.String(),
		"property_id":   propertyID.String(),
		"start_date":    "2024-01-01T00:00:00Z",
		"end_date":      "2024-12-31T00:00:00Z",
		"monthly_price": 1500000,
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
}

// Checkout tests

// TestCheckout_SetsRoomToVacant verifies that POST /v1/tenants/checkout/:id
// returns HTTP 200 with the terminated contract when checkout succeeds.
//
// Requirements: 3.2
func TestCheckout_SetsRoomToVacant(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()
	contractID := uuid.New()
	roomID := uuid.New()
	propertyID := uuid.New()

	expectedContract := dto.ContractResponse{
		ID:         contractID.String(),
		TenantID:   tenantID.String(),
		RoomID:     roomID.String(),
		PropertyID: propertyID.String(),
		Status:     "terminated",
	}

	svc := &stubTenantService{
		checkoutFn: func(_ context.Context, oID, tID uuid.UUID, _ dto.CheckoutRequest) (dto.ContractResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if tID != tenantID {
				t.Errorf("expected tenantID %s, got %s", tenantID, tID)
			}
			return expectedContract, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/checkout/"+tenantID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["status"] != "terminated" {
		t.Errorf("expected contract status=terminated, got %v", data["status"])
	}
}

// TestCheckout_Returns404WhenNoActiveContract verifies that
// POST /v1/tenants/checkout/:id returns HTTP 404 when no active contract exists.
//
// Requirements: 3.2
func TestCheckout_Returns404WhenNoActiveContract(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()

	svc := &stubTenantService{
		checkoutFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CheckoutRequest) (dto.ContractResponse, error) {
			return dto.ContractResponse{}, service.ErrNoActiveContract
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/checkout/"+tenantID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "NO_ACTIVE_CONTRACT" {
		t.Errorf("expected error code NO_ACTIVE_CONTRACT, got %v", errObj["code"])
	}
}

// TestCheckout_Returns400WhenTenantIDInvalid verifies that
// POST /v1/tenants/checkout/:id returns HTTP 400 when the tenant ID is not a valid UUID.
//
// Requirements: 3.2
func TestCheckout_Returns400WhenTenantIDInvalid(t *testing.T) {
	ownerID := uuid.New()

	svc := &stubTenantService{
		checkoutFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CheckoutRequest) (dto.ContractResponse, error) {
			t.Error("service.Checkout should not be called with invalid tenant ID")
			return dto.ContractResponse{}, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/checkout/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// Blacklist tests

// TestBlacklist_Returns400WhenReasonMissing verifies that
// POST /v1/tenants/:id/blacklist returns HTTP 400 when the reason is missing.
//
// Requirements: 3.2
func TestBlacklist_Returns400WhenReasonMissing(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()

	svc := &stubTenantService{
		blacklistFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.BlacklistRequest) (dto.TenantResponse, error) {
			t.Error("service.Blacklist should not be called when validation fails")
			return dto.TenantResponse{}, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	w := postTenantJSON(r, "/v1/tenants/"+tenantID.String()+"/blacklist", map[string]interface{}{
		// missing "reason"
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestBlacklist_Returns200OnSuccess verifies that
// POST /v1/tenants/:id/blacklist returns HTTP 200 when the blacklist succeeds.
//
// Requirements: 3.2
func TestBlacklist_Returns200OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()

	expectedTenant := dto.TenantResponse{
		ID:              tenantID.String(),
		FullName:        "Bad Tenant",
		IsBlacklisted:   true,
		BlacklistReason: "Non-payment",
	}

	svc := &stubTenantService{
		blacklistFn: func(_ context.Context, oID, tID uuid.UUID, req dto.BlacklistRequest) (dto.TenantResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if tID != tenantID {
				t.Errorf("expected tenantID %s, got %s", tenantID, tID)
			}
			if req.Reason != "Non-payment" {
				t.Errorf("expected reason=Non-payment, got %s", req.Reason)
			}
			return expectedTenant, nil
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())
	w := postTenantJSON(r, "/v1/tenants/"+tenantID.String()+"/blacklist", map[string]interface{}{
		"reason": "Non-payment",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["is_blacklisted"] != true {
		t.Errorf("expected is_blacklisted=true, got %v", data["is_blacklisted"])
	}
}
