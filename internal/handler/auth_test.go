// Package handler_test contains unit tests for the auth HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database
// or email client. A mock authServicer is injected via the interface.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func init() {
	gin.SetMode(gin.TestMode)
}

// Mock service

// mockAuthService is a test double for handler.AuthServicer.
type mockAuthService struct {
	// registerFn is called by Register; if nil the test will panic.
	registerFn func(ctx context.Context, userID uuid.UUID, req dto.RegisterRequest) (dto.ProfileResponse, error)
	// inviteFn is called by Invite.
	inviteFn func(ctx context.Context, ownerID uuid.UUID, req dto.InviteRequest) (dto.InvitationResponse, error)
	// approveFn is called by Approve.
	approveFn func(ctx context.Context, profileID uuid.UUID, email string) (dto.ProfileResponse, error)
	// rejectFn is called by Reject.
	rejectFn func(ctx context.Context, profileID uuid.UUID, email string) error
}

func (m *mockAuthService) Register(ctx context.Context, userID uuid.UUID, req dto.RegisterRequest) (dto.ProfileResponse, error) {
	return m.registerFn(ctx, userID, req)
}

func (m *mockAuthService) Invite(ctx context.Context, ownerID uuid.UUID, req dto.InviteRequest) (dto.InvitationResponse, error) {
	return m.inviteFn(ctx, ownerID, req)
}

func (m *mockAuthService) Approve(ctx context.Context, profileID uuid.UUID, email string) (dto.ProfileResponse, error) {
	return m.approveFn(ctx, profileID, email)
}

func (m *mockAuthService) Reject(ctx context.Context, profileID uuid.UUID, email string) error {
	return m.rejectFn(ctx, profileID, email)
}

// Helpers

// newRouter builds a minimal Gin router that injects userID into the context
// (simulating what the Auth middleware does) and registers the auth handler.
func newRouter(svc handler.AuthServicer, userID string) *gin.Engine {
	r := gin.New()

	// Inject the user ID into the context the same way the Auth middleware does.
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewAuthHandlerWithService(svc)
	r.POST("/auth/register", h.Register)
	r.POST("/auth/invite", h.Invite)
	r.POST("/auth/approve/:id", h.Approve)
	r.POST("/auth/reject/:id", h.Reject)

	return r
}

// postJSON performs a POST request with a JSON body and returns the recorder.
func postJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
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

// decodeBody unmarshals the response body into a map for assertion.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// Register tests

// TestRegister_CreatesProfileOnFirstCall verifies that the first call to
// POST /auth/register creates a new profile and returns it with HTTP 200.
//
// Requirements: 1.1
func TestRegister_CreatesProfileOnFirstCall(t *testing.T) {
	userID := uuid.New()
	expectedProfile := dto.ProfileResponse{
		ID:       userID.String(),
		FullName: "Budi Santoso",
		Role:     "owner",
	}

	callCount := 0
	svc := &mockAuthService{
		registerFn: func(_ context.Context, id uuid.UUID, req dto.RegisterRequest) (dto.ProfileResponse, error) {
			callCount++
			if id != userID {
				t.Errorf("expected userID %s, got %s", userID, id)
			}
			return expectedProfile, nil
		},
	}

	r := newRouter(svc, userID.String())
	w := postJSON(r, "/auth/register", map[string]string{
		"full_name": "Budi Santoso",
		"role":      "owner",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if callCount != 1 {
		t.Errorf("expected service.Register to be called once, got %d", callCount)
	}

	body := decodeBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["id"] != userID.String() {
		t.Errorf("expected id=%s, got %v", userID.String(), data["id"])
	}
	if data["full_name"] != "Budi Santoso" {
		t.Errorf("expected full_name=Budi Santoso, got %v", data["full_name"])
	}
}

// TestRegister_ReturnsExistingProfileOnSecondCall verifies that calling
// POST /auth/register a second time for the same user returns the existing
// profile (upsert semantics) with HTTP 200.
//
// Requirements: 1.1
func TestRegister_ReturnsExistingProfileOnSecondCall(t *testing.T) {
	userID := uuid.New()
	existingProfile := dto.ProfileResponse{
		ID:       userID.String(),
		FullName: "Budi Santoso",
		Role:     "owner",
	}

	callCount := 0
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ uuid.UUID, _ dto.RegisterRequest) (dto.ProfileResponse, error) {
			callCount++
			// Simulate upsert: always return the same profile regardless of call count.
			return existingProfile, nil
		},
	}

	r := newRouter(svc, userID.String())
	body := map[string]string{"full_name": "Budi Santoso", "role": "owner"}

	// First call.
	w1 := postJSON(r, "/auth/register", body)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d", w1.Code)
	}

	// Second call — same user, same data.
	w2 := postJSON(r, "/auth/register", body)
	if w2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d", w2.Code)
	}

	if callCount != 2 {
		t.Errorf("expected service.Register to be called twice, got %d", callCount)
	}

	// Both responses should return the same profile.
	b1 := decodeBody(t, w1)
	b2 := decodeBody(t, w2)

	d1, _ := b1["data"].(map[string]interface{})
	d2, _ := b2["data"].(map[string]interface{})

	if d1["id"] != d2["id"] {
		t.Errorf("expected same profile ID on both calls; got %v and %v", d1["id"], d2["id"])
	}
}

// TestRegister_Returns400WhenFullNameMissing verifies that the handler returns
// HTTP 400 when the required full_name field is absent.
//
// Requirements: 1.1
func TestRegister_Returns400WhenFullNameMissing(t *testing.T) {
	userID := uuid.New()
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ uuid.UUID, _ dto.RegisterRequest) (dto.ProfileResponse, error) {
			t.Error("service.Register should not be called when validation fails")
			return dto.ProfileResponse{}, nil
		},
	}

	r := newRouter(svc, userID.String())
	// Send body without full_name.
	w := postJSON(r, "/auth/register", map[string]string{"role": "owner"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestRegister_Returns401WhenUserIDInvalid verifies that the handler returns
// HTTP 401 when the user ID in the context is not a valid UUID.
//
// Requirements: 1.1
func TestRegister_Returns401WhenUserIDInvalid(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _ uuid.UUID, _ dto.RegisterRequest) (dto.ProfileResponse, error) {
			t.Error("service.Register should not be called with invalid user ID")
			return dto.ProfileResponse{}, nil
		},
	}

	r := newRouter(svc, "not-a-uuid")
	w := postJSON(r, "/auth/register", map[string]string{"full_name": "Test"})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// Invite tests

// TestInvite_Returns400WhenEmailMissing verifies that POST /auth/invite returns
// HTTP 400 when the required email field is absent.
//
// Requirements: 1.2
func TestInvite_Returns400WhenEmailMissing(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockAuthService{
		inviteFn: func(_ context.Context, _ uuid.UUID, _ dto.InviteRequest) (dto.InvitationResponse, error) {
			t.Error("service.Invite should not be called when email is missing")
			return dto.InvitationResponse{}, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	// Send body without email.
	w := postJSON(r, "/auth/invite", map[string]string{"property_id": uuid.New().String()})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestInvite_Returns400WhenEmailInvalid verifies that POST /auth/invite returns
// HTTP 400 when the email field is present but not a valid email address.
//
// Requirements: 1.2
func TestInvite_Returns400WhenEmailInvalid(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockAuthService{
		inviteFn: func(_ context.Context, _ uuid.UUID, _ dto.InviteRequest) (dto.InvitationResponse, error) {
			t.Error("service.Invite should not be called when email is invalid")
			return dto.InvitationResponse{}, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/invite", map[string]string{"email": "not-an-email"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestInvite_Returns201OnSuccess verifies that POST /auth/invite returns
// HTTP 201 with the invitation payload when the request is valid.
//
// Requirements: 1.2
func TestInvite_Returns201OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	invID := uuid.New()
	expectedInv := dto.InvitationResponse{
		ID:        invID.String(),
		Email:     "tenant@example.com",
		Token:     uuid.New().String(),
		ExpiresAt: "2099-01-01T00:00:00Z",
	}

	svc := &mockAuthService{
		inviteFn: func(_ context.Context, id uuid.UUID, req dto.InviteRequest) (dto.InvitationResponse, error) {
			if id != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, id)
			}
			if req.Email != "tenant@example.com" {
				t.Errorf("expected email tenant@example.com, got %s", req.Email)
			}
			return expectedInv, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/invite", map[string]string{"email": "tenant@example.com"})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["email"] != "tenant@example.com" {
		t.Errorf("expected email=tenant@example.com, got %v", data["email"])
	}
}

// Approve tests

// TestApprove_Returns404ForUnknownID verifies that POST /auth/approve/:id
// returns HTTP 404 when the profile does not exist.
//
// Requirements: 1.2
func TestApprove_Returns404ForUnknownID(t *testing.T) {
	ownerID := uuid.New()
	unknownID := uuid.New()

	svc := &mockAuthService{
		approveFn: func(_ context.Context, profileID uuid.UUID, _ string) (dto.ProfileResponse, error) {
			if profileID == unknownID {
				return dto.ProfileResponse{}, service.ErrNotFound
			}
			return dto.ProfileResponse{}, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/approve/"+unknownID.String(), nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestApprove_Returns400ForInvalidID verifies that POST /auth/approve/:id
// returns HTTP 400 when the path parameter is not a valid UUID.
//
// Requirements: 1.2
func TestApprove_Returns400ForInvalidID(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockAuthService{
		approveFn: func(_ context.Context, _ uuid.UUID, _ string) (dto.ProfileResponse, error) {
			t.Error("service.Approve should not be called with invalid ID")
			return dto.ProfileResponse{}, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/approve/not-a-uuid", nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestApprove_Returns200OnSuccess verifies that POST /auth/approve/:id
// returns HTTP 200 with the approved profile when the profile exists.
//
// Requirements: 1.2
func TestApprove_Returns200OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	profileID := uuid.New()
	expectedProfile := dto.ProfileResponse{
		ID:       profileID.String(),
		FullName: "Siti Rahayu",
		Role:     "tenant",
	}

	svc := &mockAuthService{
		approveFn: func(_ context.Context, id uuid.UUID, _ string) (dto.ProfileResponse, error) {
			if id != profileID {
				return dto.ProfileResponse{}, errors.New("unexpected profile ID")
			}
			return expectedProfile, nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/approve/"+profileID.String(), nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
}

// Reject tests

// TestReject_Returns404ForUnknownID verifies that POST /auth/reject/:id
// returns HTTP 404 when the profile does not exist.
//
// Requirements: 1.2
func TestReject_Returns404ForUnknownID(t *testing.T) {
	ownerID := uuid.New()
	unknownID := uuid.New()

	svc := &mockAuthService{
		rejectFn: func(_ context.Context, profileID uuid.UUID, _ string) error {
			if profileID == unknownID {
				return service.ErrNotFound
			}
			return nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/reject/"+unknownID.String(), nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
}

// TestReject_Returns400ForInvalidID verifies that POST /auth/reject/:id
// returns HTTP 400 when the path parameter is not a valid UUID.
//
// Requirements: 1.2
func TestReject_Returns400ForInvalidID(t *testing.T) {
	ownerID := uuid.New()
	svc := &mockAuthService{
		rejectFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			t.Error("service.Reject should not be called with invalid ID")
			return nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/reject/not-a-uuid", nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestReject_Returns200OnSuccess verifies that POST /auth/reject/:id
// returns HTTP 200 with a success message when the profile exists.
//
// Requirements: 1.2
func TestReject_Returns200OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	profileID := uuid.New()

	svc := &mockAuthService{
		rejectFn: func(_ context.Context, id uuid.UUID, _ string) error {
			if id != profileID {
				return errors.New("unexpected profile ID")
			}
			return nil
		},
	}

	r := newRouter(svc, ownerID.String())
	w := postJSON(r, "/auth/reject/"+profileID.String(), nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
}
