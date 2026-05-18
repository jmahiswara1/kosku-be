// Package dto contains request/response data transfer objects for the KosKu API.
package dto

// GenerateBillsRequest is the request body for POST /v1/bills/generate.
type GenerateBillsRequest struct {
	PropertyID    string `json:"property_id"  binding:"required,uuid"`
	PeriodMonth   int    `json:"period_month" binding:"required,min=1,max=12"`
	PeriodYear    int    `json:"period_year"  binding:"required,min=2000"`
	DueDayOfMonth int    `json:"due_day_of_month" binding:"required,min=1,max=28"`
}

// UtilityChargeItem is a single utility charge line item.
type UtilityChargeItem struct {
	Type   string  `json:"type"   binding:"required"`
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Note   string  `json:"note"`
}

// UpdateUtilitiesRequest is the request body for PUT /v1/bills/:id/utilities.
type UpdateUtilitiesRequest struct {
	Charges []UtilityChargeItem `json:"charges" binding:"required"`
}

// UtilityChargeResponse is the response for a single utility charge.
type UtilityChargeResponse struct {
	ID        string  `json:"id"`
	BillID    string  `json:"bill_id"`
	Type      string  `json:"type"`
	Amount    float64 `json:"amount"`
	Note      string  `json:"note,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

// BillResponse is the standard bill payload returned by billing endpoints.
type BillResponse struct {
	ID            string                  `json:"id"`
	TenantID      string                  `json:"tenant_id"`
	TenantName    string                  `json:"tenant_name,omitempty"`
	PropertyID    string                  `json:"property_id"`
	RoomID        string                  `json:"room_id"`
	PeriodMonth   int                     `json:"period_month"`
	PeriodYear    int                     `json:"period_year"`
	BaseAmount    string                  `json:"base_amount"`
	UtilityAmount string                  `json:"utility_amount"`
	PenaltyAmount string                  `json:"penalty_amount"`
	TotalAmount   string                  `json:"total_amount"`
	DueDate       string                  `json:"due_date"`
	Status        string                  `json:"status"`
	Charges       []UtilityChargeResponse `json:"charges,omitempty"`
	CreatedAt     string                  `json:"created_at,omitempty"`
	UpdatedAt     string                  `json:"updated_at,omitempty"`
}

// SubmitPaymentRequest is the request body for POST /v1/payments.
// The proof image is submitted as a multipart form field named "proof".
type SubmitPaymentRequest struct {
	BillID string  `form:"bill_id" binding:"required,uuid"`
	Amount float64 `form:"amount"  binding:"required,gt=0"`
}

// ConfirmPaymentRequest is the request body for PUT /v1/payments/:id/confirm.
type ConfirmPaymentRequest struct {
	// No additional fields required — the confirmer is taken from the JWT.
}

// RejectPaymentRequest is the request body for PUT /v1/payments/:id/reject.
type RejectPaymentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// PaymentResponse is the standard payment payload returned by payment endpoints.
type PaymentResponse struct {
	ID              string `json:"id"`
	BillID          string `json:"bill_id"`
	TenantID        string `json:"tenant_id"`
	Amount          string `json:"amount"`
	ProofURL        string `json:"proof_url,omitempty"`
	Status          string `json:"status"`
	RejectionReason string `json:"rejection_reason,omitempty"`
	ConfirmedBy     string `json:"confirmed_by,omitempty"`
	ConfirmedAt     string `json:"confirmed_at,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// FinancialReportRow is a single row in the financial report.
type FinancialReportRow struct {
	PropertyID  string `json:"property_id"`
	PeriodMonth int    `json:"period_month"`
	PeriodYear  int    `json:"period_year"`
	TotalBilled int64  `json:"total_billed"`
	TotalPaid   int64  `json:"total_paid"`
	BillCount   int64  `json:"bill_count"`
}

// FinancialReportResponse is the response for GET /v1/reports/financial.
type FinancialReportResponse struct {
	PropertyID string               `json:"property_id"`
	FromMonth  int                  `json:"from_month"`
	FromYear   int                  `json:"from_year"`
	ToMonth    int                  `json:"to_month"`
	ToYear     int                  `json:"to_year"`
	Rows       []FinancialReportRow `json:"rows"`
}
