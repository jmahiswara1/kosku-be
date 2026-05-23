// Package handler_test contains unit tests for the billing HTTP handlers.
// Tests use httptest to exercise the Gin handlers without a real database.
// A mock BillingServicer is injected via the interface.
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

// stubBillingService is a test double for handler.BillingServicer.
type stubBillingService struct {
	generateBillsFn      func(ctx context.Context, ownerID uuid.UUID, req dto.GenerateBillsRequest) ([]dto.BillResponse, error)
	listBillsFn          func(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, tenantName, fromDate, toDate string, page, perPage int) ([]dto.BillResponse, int64, error)
	getBillFn            func(ctx context.Context, ownerID, billID uuid.UUID) (dto.BillResponse, error)
	updateUtilitiesFn    func(ctx context.Context, ownerID, billID uuid.UUID, req dto.UpdateUtilitiesRequest) (dto.BillResponse, error)
	submitPaymentFn      func(ctx context.Context, tenantID uuid.UUID, req dto.SubmitPaymentRequest, fileData []byte, declaredContentType string) (dto.PaymentResponse, error)
	confirmPaymentFn     func(ctx context.Context, ownerID, paymentID uuid.UUID) (dto.PaymentResponse, error)
	rejectPaymentFn      func(ctx context.Context, ownerID, paymentID uuid.UUID, req dto.RejectPaymentRequest) (dto.PaymentResponse, error)
	getFinancialReportFn func(ctx context.Context, ownerID, propertyID uuid.UUID, fromMonth, fromYear, toMonth, toYear int) (dto.FinancialReportResponse, error)
}

func (m *stubBillingService) GenerateBills(ctx context.Context, ownerID uuid.UUID, req dto.GenerateBillsRequest) ([]dto.BillResponse, error) {
	return m.generateBillsFn(ctx, ownerID, req)
}

func (m *stubBillingService) ListBills(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, tenantName, fromDate, toDate string, page, perPage int) ([]dto.BillResponse, int64, error) {
	return m.listBillsFn(ctx, ownerID, propertyID, status, tenantName, fromDate, toDate, page, perPage)
}

func (m *stubBillingService) GetBill(ctx context.Context, ownerID, billID uuid.UUID) (dto.BillResponse, error) {
	return m.getBillFn(ctx, ownerID, billID)
}

func (m *stubBillingService) UpdateUtilities(ctx context.Context, ownerID, billID uuid.UUID, req dto.UpdateUtilitiesRequest) (dto.BillResponse, error) {
	return m.updateUtilitiesFn(ctx, ownerID, billID, req)
}

func (m *stubBillingService) SubmitPayment(ctx context.Context, tenantID uuid.UUID, req dto.SubmitPaymentRequest, fileData []byte, declaredContentType string) (dto.PaymentResponse, error) {
	return m.submitPaymentFn(ctx, tenantID, req, fileData, declaredContentType)
}

func (m *stubBillingService) ConfirmPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (dto.PaymentResponse, error) {
	return m.confirmPaymentFn(ctx, ownerID, paymentID)
}

func (m *stubBillingService) RejectPayment(ctx context.Context, ownerID, paymentID uuid.UUID, req dto.RejectPaymentRequest) (dto.PaymentResponse, error) {
	return m.rejectPaymentFn(ctx, ownerID, paymentID, req)
}

func (m *stubBillingService) GetFinancialReport(ctx context.Context, ownerID, propertyID uuid.UUID, fromMonth, fromYear, toMonth, toYear int) (dto.FinancialReportResponse, error) {
	return m.getFinancialReportFn(ctx, ownerID, propertyID, fromMonth, fromYear, toMonth, toYear)
}

// Helpers

// newBillingHandlerRouter builds a minimal Gin router with the billing handler routes.
func newBillingHandlerRouter(svc handler.BillingServicer, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		c.Next()
	})

	h := handler.NewBillingHandlerWithService(svc)
	r.POST("/v1/bills/generate", h.GenerateBills)
	r.GET("/v1/bills", h.ListBills)
	r.GET("/v1/bills/:id", h.GetBill)
	r.PUT("/v1/bills/:id/utilities", h.UpdateUtilities)
	r.PUT("/v1/payments/:id/confirm", h.ConfirmPayment)
	r.PUT("/v1/payments/:id/reject", h.RejectPayment)
	r.GET("/v1/reports/financial", h.GetFinancialReport)
	r.GET("/v1/reports/financial/export", h.ExportFinancialReport)

	return r
}

// postBillingJSON performs a POST request with a JSON body.
func postBillingJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
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

// putBillingJSON performs a PUT request with a JSON body.
func putBillingJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeBillingBody unmarshals the response body into a map.
func decodeBillingBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// GenerateBills tests

// TestGenerateBills_CreatesOneBillPerActiveContract verifies that POST /v1/bills/generate
// returns HTTP 201 with the generated bills.
//
// Requirements: 8.1
func TestGenerateBills_CreatesOneBillPerActiveContract(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	expectedBills := []dto.BillResponse{
		{ID: uuid.New().String(), PropertyID: propertyID.String(), Status: "unpaid"},
		{ID: uuid.New().String(), PropertyID: propertyID.String(), Status: "unpaid"},
		{ID: uuid.New().String(), PropertyID: propertyID.String(), Status: "unpaid"},
	}

	svc := &stubBillingService{
		generateBillsFn: func(_ context.Context, oID uuid.UUID, req dto.GenerateBillsRequest) ([]dto.BillResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if req.PropertyID != propertyID.String() {
				t.Errorf("expected propertyID %s, got %s", propertyID, req.PropertyID)
			}
			return expectedBills, nil
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	w := postBillingJSON(r, "/v1/bills/generate", map[string]interface{}{
		"property_id":      propertyID.String(),
		"period_month":     1,
		"period_year":      2025,
		"due_day_of_month": 15,
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBillingBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", body["data"])
	}
	if len(data) != 3 {
		t.Errorf("expected 3 bills, got %d", len(data))
	}

	meta, ok := body["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected meta to be an object, got %T", body["meta"])
	}
	if meta["generated"] != float64(3) {
		t.Errorf("expected meta.generated=3, got %v", meta["generated"])
	}
}

// TestGenerateBills_Returns400WhenRequiredFieldsMissing verifies that
// POST /v1/bills/generate returns HTTP 400 when required fields are absent.
//
// Requirements: 8.1
func TestGenerateBills_Returns400WhenRequiredFieldsMissing(t *testing.T) {
	ownerID := uuid.New()

	svc := &stubBillingService{
		generateBillsFn: func(_ context.Context, _ uuid.UUID, _ dto.GenerateBillsRequest) ([]dto.BillResponse, error) {
			t.Error("service.GenerateBills should not be called when validation fails")
			return nil, nil
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	// Missing required fields.
	w := postBillingJSON(r, "/v1/bills/generate", map[string]interface{}{
		"period_month": 1,
		// missing property_id, period_year, due_day_of_month
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestGenerateBills_Returns404WhenPropertyNotFound verifies that
// POST /v1/bills/generate returns HTTP 404 when the property is not found.
//
// Requirements: 8.1
func TestGenerateBills_Returns404WhenPropertyNotFound(t *testing.T) {
	ownerID := uuid.New()
	propertyID := uuid.New()

	svc := &stubBillingService{
		generateBillsFn: func(_ context.Context, _ uuid.UUID, _ dto.GenerateBillsRequest) ([]dto.BillResponse, error) {
			return nil, service.ErrNotFound
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	w := postBillingJSON(r, "/v1/bills/generate", map[string]interface{}{
		"property_id":      propertyID.String(),
		"period_month":     1,
		"period_year":      2025,
		"due_day_of_month": 15,
	})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ConfirmPayment tests

// TestConfirmPayment_RejectsIfAlreadyConfirmed verifies that
// PUT /v1/payments/:id/confirm returns HTTP 409 when the payment is already confirmed.
//
// Requirements: 7.2
func TestConfirmPayment_RejectsIfAlreadyConfirmed(t *testing.T) {
	ownerID := uuid.New()
	paymentID := uuid.New()

	svc := &stubBillingService{
		confirmPaymentFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (dto.PaymentResponse, error) {
			return dto.PaymentResponse{}, service.ErrPaymentAlreadyConfirmed
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPut, "/v1/payments/"+paymentID.String()+"/confirm", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBillingBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "PAYMENT_ALREADY_CONFIRMED" {
		t.Errorf("expected error code PAYMENT_ALREADY_CONFIRMED, got %v", errObj["code"])
	}
}

// TestConfirmPayment_Returns200OnSuccess verifies that
// PUT /v1/payments/:id/confirm returns HTTP 200 when confirmation succeeds.
//
// Requirements: 7.2
func TestConfirmPayment_Returns200OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	paymentID := uuid.New()
	billID := uuid.New()

	expectedPayment := dto.PaymentResponse{
		ID:     paymentID.String(),
		BillID: billID.String(),
		Status: "confirmed",
	}

	svc := &stubBillingService{
		confirmPaymentFn: func(_ context.Context, oID, pID uuid.UUID) (dto.PaymentResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if pID != paymentID {
				t.Errorf("expected paymentID %s, got %s", paymentID, pID)
			}
			return expectedPayment, nil
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPut, "/v1/payments/"+paymentID.String()+"/confirm", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBillingBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["status"] != "confirmed" {
		t.Errorf("expected status=confirmed, got %v", data["status"])
	}
}

// TestConfirmPayment_Returns404WhenNotFound verifies that
// PUT /v1/payments/:id/confirm returns HTTP 404 when the payment is not found.
//
// Requirements: 7.2
func TestConfirmPayment_Returns404WhenNotFound(t *testing.T) {
	ownerID := uuid.New()
	paymentID := uuid.New()

	svc := &stubBillingService{
		confirmPaymentFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (dto.PaymentResponse, error) {
			return dto.PaymentResponse{}, service.ErrNotFound
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	req := httptest.NewRequest(http.MethodPut, "/v1/payments/"+paymentID.String()+"/confirm", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}
}

// RejectPayment tests

// TestRejectPayment_Returns400WhenReasonMissing verifies that
// PUT /v1/payments/:id/reject returns HTTP 400 when the reason is missing.
//
// Requirements: 7.2
func TestRejectPayment_Returns400WhenReasonMissing(t *testing.T) {
	ownerID := uuid.New()
	paymentID := uuid.New()

	svc := &stubBillingService{
		rejectPaymentFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.RejectPaymentRequest) (dto.PaymentResponse, error) {
			t.Error("service.RejectPayment should not be called when validation fails")
			return dto.PaymentResponse{}, nil
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	w := putBillingJSON(r, "/v1/payments/"+paymentID.String()+"/reject", map[string]interface{}{
		// missing "reason"
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestRejectPayment_Returns200OnSuccess verifies that
// PUT /v1/payments/:id/reject returns HTTP 200 when rejection succeeds.
//
// Requirements: 7.2
func TestRejectPayment_Returns200OnSuccess(t *testing.T) {
	ownerID := uuid.New()
	paymentID := uuid.New()

	expectedPayment := dto.PaymentResponse{
		ID:              paymentID.String(),
		Status:          "rejected",
		RejectionReason: "Proof image is unclear",
	}

	svc := &stubBillingService{
		rejectPaymentFn: func(_ context.Context, oID, pID uuid.UUID, req dto.RejectPaymentRequest) (dto.PaymentResponse, error) {
			if oID != ownerID {
				t.Errorf("expected ownerID %s, got %s", ownerID, oID)
			}
			if pID != paymentID {
				t.Errorf("expected paymentID %s, got %s", paymentID, pID)
			}
			if req.Reason != "Proof image is unclear" {
				t.Errorf("expected reason='Proof image is unclear', got %s", req.Reason)
			}
			return expectedPayment, nil
		},
	}

	r := newBillingHandlerRouter(svc, ownerID.String())
	w := putBillingJSON(r, "/v1/payments/"+paymentID.String()+"/reject", map[string]interface{}{
		"reason": "Proof image is unclear",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeBillingBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body["data"])
	}
	if data["status"] != "rejected" {
		t.Errorf("expected status=rejected, got %v", data["status"])
	}
}

// Deposit refund tests (via tenant checkout handler)

// TestCheckout_RejectsIfRefundExceedsDeposit verifies that
// POST /v1/tenants/checkout/:id returns HTTP 400 when refund > deposit.
//
// Requirements: 8.2
func TestCheckout_RejectsIfRefundExceedsDeposit(t *testing.T) {
	ownerID := uuid.New()
	tenantID := uuid.New()

	svc := &stubTenantService{
		checkoutFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ dto.CheckoutRequest) (dto.ContractResponse, error) {
			return dto.ContractResponse{}, service.ErrRefundExceedsDeposit
		},
	}

	r := newTenantHandlerRouter(svc, ownerID.String())

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]interface{}{
		"refund_amount": 9999999.99,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/checkout/"+tenantID.String(), &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}

	body := decodeTenantBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}

	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error to be an object, got %T", body["error"])
	}
	if errObj["code"] != "REFUND_EXCEEDS_DEPOSIT" {
		t.Errorf("expected error code REFUND_EXCEEDS_DEPOSIT, got %v", errObj["code"])
	}
}
