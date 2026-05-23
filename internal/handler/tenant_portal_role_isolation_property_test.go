// Package handler_test contains property-based tests for the tenant portal HTTP handlers.
//
// Validates: Requirements 1.3, 12
//
// Property 3: Role isolation
// For any tenant user ID, calling GET /v1/me/bills, GET /v1/me/room, and
// GET /v1/me/tickets must never return records belonging to a different tenant
// or a property the tenant is not assigned to.
//
// This test verifies the isolation invariant: the service layer is always
// called with the authenticated tenant's ID (extracted from the JWT context),
// and the handler never leaks another tenant's data by passing a different ID
// to the service.
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/quick"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/handler"
	"github.com/kosku/backend/internal/middleware"
)

// Mock tenant portal service for role isolation tests

// mockTenantPortalService is a test double for handler.TenantPortalServicer.
// Each field holds the function that will be called for the corresponding method.
type mockTenantPortalService struct {
	getMyRoomFn              func(ctx context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error)
	listMyBillsFn            func(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.BillResponse, int64, error)
	getBillReceiptFn         func(ctx context.Context, tenantID, billID uuid.UUID) ([]byte, string, error)
	listMyTicketsFn          func(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.TicketResponse, int64, error)
	createMyTicketFn         func(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error)
	listMyContractsFn        func(ctx context.Context, tenantID uuid.UUID) ([]dto.ContractResponse, error)
	requestContractRenewalFn func(ctx context.Context, tenantID uuid.UUID, req dto.ContractRenewalRequest) error
}

func (m *mockTenantPortalService) GetMyRoom(ctx context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error) {
	return m.getMyRoomFn(ctx, tenantID)
}

func (m *mockTenantPortalService) ListMyBills(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.BillResponse, int64, error) {
	return m.listMyBillsFn(ctx, tenantID, status, page, perPage)
}

func (m *mockTenantPortalService) GetBillReceipt(ctx context.Context, tenantID, billID uuid.UUID) ([]byte, string, error) {
	return m.getBillReceiptFn(ctx, tenantID, billID)
}

func (m *mockTenantPortalService) ListMyTickets(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.TicketResponse, int64, error) {
	return m.listMyTicketsFn(ctx, tenantID, status, page, perPage)
}

func (m *mockTenantPortalService) CreateMyTicket(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error) {
	return m.createMyTicketFn(ctx, tenantID, req, photos, photoContentTypes)
}

func (m *mockTenantPortalService) ListMyContracts(ctx context.Context, tenantID uuid.UUID) ([]dto.ContractResponse, error) {
	return m.listMyContractsFn(ctx, tenantID)
}

func (m *mockTenantPortalService) RequestContractRenewal(ctx context.Context, tenantID uuid.UUID, req dto.ContractRenewalRequest) error {
	return m.requestContractRenewalFn(ctx, tenantID, req)
}

// Router helper for tenant portal tests

// newTenantPortalRouter builds a minimal Gin router that injects tenantID into
// the context (simulating what the Auth middleware does) and registers the
// tenant portal handler routes needed for role isolation tests.
func newTenantPortalRouter(svc handler.TenantPortalServicer, tenantID string) *gin.Engine {
	r := gin.New()

	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, tenantID)
		c.Next()
	})

	h := handler.NewTenantPortalHandlerWithService(svc)
	r.GET("/v1/me/room", h.GetMyRoom)
	r.GET("/v1/me/bills", h.ListMyBills)
	r.GET("/v1/me/tickets", h.ListMyTickets)

	return r
}

// doGet performs a GET request and returns the recorder.
func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Property 3: Role isolation

// TestRoleIsolation_BillsNeverLeakOtherTenantData is a property-based test
// that verifies GET /v1/me/bills always queries with the authenticated tenant's
// ID and never returns bills belonging to a different tenant.
//
// For any pair of distinct tenant UUIDs (authenticatedTenant, otherTenant),
// the service is called with exactly authenticatedTenant, and the returned
// bills all carry authenticatedTenant's ID — never otherTenant's.
//
// Validates: Requirements 1.3, 12
func TestRoleIsolation_BillsNeverLeakOtherTenantData(t *testing.T) {
	// property verifies the isolation invariant for GET /v1/me/bills.
	// testing/quick generates random [16]byte arrays for the two tenant IDs.
	property := func(rawA [16]byte, rawB [16]byte) bool {
		authenticatedTenant := uuid.UUID(rawA)
		otherTenant := uuid.UUID(rawB)

		// Ensure the two tenants are distinct; skip degenerate case.
		if authenticatedTenant == otherTenant {
			return true
		}

		// The service records which tenantID it was called with.
		var calledWithID uuid.UUID

		// Build bills that belong to the authenticated tenant.
		ownBills := []dto.BillResponse{
			{ID: uuid.New().String(), TenantID: authenticatedTenant.String(), PropertyID: uuid.New().String(), Status: "unpaid"},
			{ID: uuid.New().String(), TenantID: authenticatedTenant.String(), PropertyID: uuid.New().String(), Status: "paid"},
		}

		svc := &mockTenantPortalService{
			listMyBillsFn: func(_ context.Context, tenantID uuid.UUID, _ string, _, _ int) ([]dto.BillResponse, int64, error) {
				calledWithID = tenantID
				// The service correctly scopes to the given tenantID.
				// Return only bills for the authenticated tenant.
				return ownBills, int64(len(ownBills)), nil
			},
		}

		router := newTenantPortalRouter(svc, authenticatedTenant.String())
		w := doGet(router, "/v1/me/bills")

		if w.Code != http.StatusOK {
			t.Logf("GET /v1/me/bills returned %d: %s", w.Code, w.Body.String())
			return false
		}

		// Invariant 1: the service must be called with the authenticated tenant's ID.
		if calledWithID != authenticatedTenant {
			t.Logf("service called with tenantID %s, expected %s", calledWithID, authenticatedTenant)
			return false
		}

		// Invariant 2: the service must NOT be called with the other tenant's ID.
		if calledWithID == otherTenant {
			t.Logf("service was called with otherTenant ID %s — isolation violated", otherTenant)
			return false
		}

		// Invariant 3: all returned bills must belong to the authenticated tenant.
		var resp struct {
			Success bool               `json:"success"`
			Data    []dto.BillResponse `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Logf("failed to decode response: %v", err)
			return false
		}
		if !resp.Success {
			t.Logf("response success=false")
			return false
		}
		for _, bill := range resp.Data {
			if bill.TenantID != authenticatedTenant.String() {
				t.Logf("bill %s has tenantID %s, expected %s — isolation violated",
					bill.ID, bill.TenantID, authenticatedTenant)
				return false
			}
			if bill.TenantID == otherTenant.String() {
				t.Logf("bill %s belongs to otherTenant %s — isolation violated",
					bill.ID, otherTenant)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("role isolation property violated for GET /v1/me/bills: %v", err)
	}
}

// TestRoleIsolation_RoomNeverLeakOtherTenantData is a property-based test
// that verifies GET /v1/me/room always queries with the authenticated tenant's
// ID and never returns room data belonging to a different tenant.
//
// Validates: Requirements 1.3, 12
func TestRoleIsolation_RoomNeverLeakOtherTenantData(t *testing.T) {
	property := func(rawA [16]byte, rawB [16]byte) bool {
		authenticatedTenant := uuid.UUID(rawA)
		otherTenant := uuid.UUID(rawB)

		if authenticatedTenant == otherTenant {
			return true
		}

		var calledWithID uuid.UUID

		ownPropertyID := uuid.New()
		ownRoomID := uuid.New()

		ownRoom := dto.TenantRoomResponse{
			RoomID:     ownRoomID.String(),
			RoomNumber: "101",
			Status:     "occupied",
			PropertyID: ownPropertyID.String(),
		}

		svc := &mockTenantPortalService{
			getMyRoomFn: func(_ context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error) {
				calledWithID = tenantID
				return ownRoom, nil
			},
		}

		router := newTenantPortalRouter(svc, authenticatedTenant.String())
		w := doGet(router, "/v1/me/room")

		if w.Code != http.StatusOK {
			t.Logf("GET /v1/me/room returned %d: %s", w.Code, w.Body.String())
			return false
		}

		// Invariant 1: the service must be called with the authenticated tenant's ID.
		if calledWithID != authenticatedTenant {
			t.Logf("service called with tenantID %s, expected %s", calledWithID, authenticatedTenant)
			return false
		}

		// Invariant 2: the service must NOT be called with the other tenant's ID.
		if calledWithID == otherTenant {
			t.Logf("service was called with otherTenant ID %s — isolation violated", otherTenant)
			return false
		}

		// Invariant 3: the returned room must belong to the authenticated tenant's property,
		// not to a property the other tenant might be assigned to.
		var resp struct {
			Success bool                   `json:"success"`
			Data    dto.TenantRoomResponse `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Logf("failed to decode response: %v", err)
			return false
		}
		if !resp.Success {
			t.Logf("response success=false")
			return false
		}
		// The room returned must be the one the service returned for the authenticated tenant.
		if resp.Data.RoomID != ownRoomID.String() {
			t.Logf("returned roomID %s does not match expected %s — isolation violated",
				resp.Data.RoomID, ownRoomID)
			return false
		}
		if resp.Data.PropertyID != ownPropertyID.String() {
			t.Logf("returned propertyID %s does not match expected %s — isolation violated",
				resp.Data.PropertyID, ownPropertyID)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("role isolation property violated for GET /v1/me/room: %v", err)
	}
}

// TestRoleIsolation_TicketsNeverLeakOtherTenantData is a property-based test
// that verifies GET /v1/me/tickets always queries with the authenticated tenant's
// ID and never returns tickets submitted by a different tenant.
//
// Validates: Requirements 1.3, 12
func TestRoleIsolation_TicketsNeverLeakOtherTenantData(t *testing.T) {
	property := func(rawA [16]byte, rawB [16]byte) bool {
		authenticatedTenant := uuid.UUID(rawA)
		otherTenant := uuid.UUID(rawB)

		if authenticatedTenant == otherTenant {
			return true
		}

		var calledWithID uuid.UUID

		ownPropertyID := uuid.New()
		ownTickets := []dto.TicketResponse{
			{
				ID:          uuid.New().String(),
				TenantID:    authenticatedTenant.String(),
				PropertyID:  ownPropertyID.String(),
				Title:       "Broken AC",
				Description: "AC not working",
				Priority:    "medium",
				Status:      "open",
			},
		}

		svc := &mockTenantPortalService{
			listMyTicketsFn: func(_ context.Context, tenantID uuid.UUID, _ string, _, _ int) ([]dto.TicketResponse, int64, error) {
				calledWithID = tenantID
				return ownTickets, int64(len(ownTickets)), nil
			},
		}

		router := newTenantPortalRouter(svc, authenticatedTenant.String())
		w := doGet(router, "/v1/me/tickets")

		if w.Code != http.StatusOK {
			t.Logf("GET /v1/me/tickets returned %d: %s", w.Code, w.Body.String())
			return false
		}

		// Invariant 1: the service must be called with the authenticated tenant's ID.
		if calledWithID != authenticatedTenant {
			t.Logf("service called with tenantID %s, expected %s", calledWithID, authenticatedTenant)
			return false
		}

		// Invariant 2: the service must NOT be called with the other tenant's ID.
		if calledWithID == otherTenant {
			t.Logf("service was called with otherTenant ID %s — isolation violated", otherTenant)
			return false
		}

		// Invariant 3: all returned tickets must belong to the authenticated tenant.
		var resp struct {
			Success bool                 `json:"success"`
			Data    []dto.TicketResponse `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Logf("failed to decode response: %v", err)
			return false
		}
		if !resp.Success {
			t.Logf("response success=false")
			return false
		}
		for _, ticket := range resp.Data {
			if ticket.TenantID != authenticatedTenant.String() {
				t.Logf("ticket %s has tenantID %s, expected %s — isolation violated",
					ticket.ID, ticket.TenantID, authenticatedTenant)
				return false
			}
			if ticket.TenantID == otherTenant.String() {
				t.Logf("ticket %s belongs to otherTenant %s — isolation violated",
					ticket.ID, otherTenant)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("role isolation property violated for GET /v1/me/tickets: %v", err)
	}
}

// Property 3: Role isolation — cross-tenant injection attempt

// TestRoleIsolation_HandlerCannotBeManipulatedToUseOtherTenantID verifies that
// even if the service is set up with data for multiple tenants, the handler
// always passes the authenticated tenant's ID from the JWT context — never a
// tenant ID that might be supplied via query parameters or other means.
//
// This test simulates an adversarial scenario where an attacker attempts to
// retrieve another tenant's data by manipulating the request.
//
// Validates: Requirements 1.3, 12
func TestRoleIsolation_HandlerCannotBeManipulatedToUseOtherTenantID(t *testing.T) {
	property := func(rawA [16]byte, rawB [16]byte) bool {
		authenticatedTenant := uuid.UUID(rawA)
		otherTenant := uuid.UUID(rawB)

		if authenticatedTenant == otherTenant {
			return true
		}

		// Track all tenant IDs the service was called with.
		var billsCalledWith, roomCalledWith, ticketsCalledWith uuid.UUID

		svc := &mockTenantPortalService{
			listMyBillsFn: func(_ context.Context, tenantID uuid.UUID, _ string, _, _ int) ([]dto.BillResponse, int64, error) {
				billsCalledWith = tenantID
				return []dto.BillResponse{
					{ID: uuid.New().String(), TenantID: tenantID.String(), PropertyID: uuid.New().String(), Status: "unpaid"},
				}, 1, nil
			},
			getMyRoomFn: func(_ context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error) {
				roomCalledWith = tenantID
				return dto.TenantRoomResponse{
					RoomID:     uuid.New().String(),
					RoomNumber: "101",
					Status:     "occupied",
					PropertyID: uuid.New().String(),
				}, nil
			},
			listMyTicketsFn: func(_ context.Context, tenantID uuid.UUID, _ string, _, _ int) ([]dto.TicketResponse, int64, error) {
				ticketsCalledWith = tenantID
				return []dto.TicketResponse{
					{
						ID:          uuid.New().String(),
						TenantID:    tenantID.String(),
						PropertyID:  uuid.New().String(),
						Title:       "Test",
						Description: "Test",
						Priority:    "low",
						Status:      "open",
					},
				}, 1, nil
			},
		}

		// The router is configured with the authenticated tenant's ID in context.
		// An attacker might try to pass the other tenant's ID as a query param,
		// but the handler must ignore it and use only the context ID.
		router := newTenantPortalRouter(svc, authenticatedTenant.String())

		// Attempt to inject the other tenant's ID via query parameter (adversarial).
		billsW := doGet(router, "/v1/me/bills?tenant_id="+otherTenant.String())
		roomW := doGet(router, "/v1/me/room?tenant_id="+otherTenant.String())
		ticketsW := doGet(router, "/v1/me/tickets?tenant_id="+otherTenant.String())

		if billsW.Code != http.StatusOK || roomW.Code != http.StatusOK || ticketsW.Code != http.StatusOK {
			t.Logf("unexpected status: bills=%d room=%d tickets=%d",
				billsW.Code, roomW.Code, ticketsW.Code)
			return false
		}

		// All three handlers must have called the service with the authenticated
		// tenant's ID — never with the injected otherTenant ID.
		if billsCalledWith != authenticatedTenant {
			t.Logf("bills: service called with %s, expected %s", billsCalledWith, authenticatedTenant)
			return false
		}
		if billsCalledWith == otherTenant {
			t.Logf("bills: service called with otherTenant %s — injection succeeded, isolation violated", otherTenant)
			return false
		}

		if roomCalledWith != authenticatedTenant {
			t.Logf("room: service called with %s, expected %s", roomCalledWith, authenticatedTenant)
			return false
		}
		if roomCalledWith == otherTenant {
			t.Logf("room: service called with otherTenant %s — injection succeeded, isolation violated", otherTenant)
			return false
		}

		if ticketsCalledWith != authenticatedTenant {
			t.Logf("tickets: service called with %s, expected %s", ticketsCalledWith, authenticatedTenant)
			return false
		}
		if ticketsCalledWith == otherTenant {
			t.Logf("tickets: service called with otherTenant %s — injection succeeded, isolation violated", otherTenant)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("role isolation cross-tenant injection property violated: %v", err)
	}
}
