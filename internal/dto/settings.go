// Package dto contains request/response data transfer objects for the KosKu API.
package dto

// SettingsResponse is the combined settings response for GET /v1/settings.
type SettingsResponse struct {
	PropertyID      string  `json:"property_id"`
	Name            string  `json:"name"`
	Address         string  `json:"address"`
	City            string  `json:"city,omitempty"`
	LogoURL         string  `json:"logo_url,omitempty"`
	Phone           string  `json:"phone,omitempty"`
	BankName        string  `json:"bank_name,omitempty"`
	BankAccount     string  `json:"bank_account,omitempty"`
	DueDateDay      int     `json:"due_date_day,omitempty"`
	GracePeriodDays int     `json:"grace_period_days,omitempty"`
	PenaltyType     string  `json:"penalty_type,omitempty"`
	PenaltyAmount   float64 `json:"penalty_amount,omitempty"`
}

// UpdateProfileSettingsRequest is the request body for PUT /v1/settings/profile.
type UpdateProfileSettingsRequest struct {
	Name        string `json:"name"    binding:"required"`
	Address     string `json:"address" binding:"required"`
	City        string `json:"city"`
	LogoURL     string `json:"logo_url"`
	Phone       string `json:"phone"`
	BankName    string `json:"bank_name"`
	BankAccount string `json:"bank_account"`
}

// UpdateBillingSettingsRequest is the request body for PUT /v1/settings/billing.
type UpdateBillingSettingsRequest struct {
	DueDateDay      int     `json:"due_date_day"      binding:"required,min=1,max=28"`
	GracePeriodDays int     `json:"grace_period_days" binding:"min=0"`
	PenaltyType     string  `json:"penalty_type"      binding:"omitempty,oneof=flat percentage"`
	PenaltyAmount   float64 `json:"penalty_amount"    binding:"min=0"`
}

// StaffResponse is the response for a single staff member.
type StaffResponse struct {
	ID        string   `json:"id"`
	StaffID   string   `json:"staff_id"`
	OwnerID   string   `json:"owner_id"`
	FullName  string   `json:"full_name"`
	Phone     string   `json:"phone,omitempty"`
	AvatarURL string   `json:"avatar_url,omitempty"`
	Modules   []string `json:"modules"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// AddStaffRequest is the request body for POST /v1/settings/staff.
type AddStaffRequest struct {
	Email   string   `json:"email"   binding:"required,email"`
	Modules []string `json:"modules" binding:"required"`
}

// AuditLogResponse is the response for a single audit log entry.
type AuditLogResponse struct {
	ID         string      `json:"id"`
	ActorID    string      `json:"actor_id"`
	Action     string      `json:"action"`
	EntityType string      `json:"entity_type"`
	EntityID   string      `json:"entity_id,omitempty"`
	Metadata   interface{} `json:"metadata,omitempty"`
	CreatedAt  string      `json:"created_at,omitempty"`
}
