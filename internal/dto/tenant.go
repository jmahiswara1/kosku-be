package dto

import "time"

// CheckinRequest is the request body for POST /v1/tenants/checkin.
type CheckinRequest struct {
	TenantID      string    `json:"tenant_id"      binding:"required,uuid"`
	RoomID        string    `json:"room_id"        binding:"required,uuid"`
	PropertyID    string    `json:"property_id"    binding:"required,uuid"`
	StartDate     time.Time `json:"start_date"     binding:"required"`
	EndDate       time.Time `json:"end_date"       binding:"required"`
	MonthlyPrice  float64   `json:"monthly_price"  binding:"required,gt=0"`
	DepositAmount float64   `json:"deposit_amount"`
}

// CheckoutRequest is the request body for POST /v1/tenants/checkout/:id.
type CheckoutRequest struct {
	CheckoutDate time.Time `json:"checkout_date"`
	RefundAmount float64   `json:"refund_amount"`
}

// BlacklistRequest is the request body for POST /v1/tenants/:id/blacklist.
type BlacklistRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// UpdateTenantRequest is the request body for PUT /v1/tenants/:id.
type UpdateTenantRequest struct {
	FullName       *string `json:"full_name"`
	Phone          *string `json:"phone"`
	KTPNumber      *string `json:"ktp_number"`
	Occupation     *string `json:"occupation"`
	EmergencyName  *string `json:"emergency_name"`
	EmergencyPhone *string `json:"emergency_phone"`
}

// TenantResponse is the standard tenant payload returned by tenant endpoints.
type TenantResponse struct {
	ID              string `json:"id"`
	PropertyID      string `json:"property_id,omitempty"`
	RoomID          string `json:"room_id,omitempty"`
	FullName        string `json:"full_name"`
	Phone           string `json:"phone,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	KTPNumber       string `json:"ktp_number,omitempty"`
	KTPScanURL      string `json:"ktp_scan_url,omitempty"`
	Occupation      string `json:"occupation,omitempty"`
	EmergencyName   string `json:"emergency_name,omitempty"`
	EmergencyPhone  string `json:"emergency_phone,omitempty"`
	IsBlacklisted   bool   `json:"is_blacklisted"`
	BlacklistReason string `json:"blacklist_reason,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// ContractResponse is the standard contract payload returned by tenant endpoints.
type ContractResponse struct {
	ID              string `json:"id"`
	TenantID        string `json:"tenant_id"`
	RoomID          string `json:"room_id"`
	PropertyID      string `json:"property_id"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	MonthlyPrice    string `json:"monthly_price"`
	DepositAmount   string `json:"deposit_amount,omitempty"`
	DepositRefunded string `json:"deposit_refunded,omitempty"`
	Status          string `json:"status"`
	FileURL         string `json:"file_url,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// CheckinResponse is the response for a successful check-in.
type CheckinResponse struct {
	Tenant   TenantResponse   `json:"tenant"`
	Contract ContractResponse `json:"contract"`
}
