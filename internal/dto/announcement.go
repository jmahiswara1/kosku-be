package dto

// CreateAnnouncementRequest is the request body for POST /v1/announcements.
type CreateAnnouncementRequest struct {
	Title      string   `json:"title" binding:"required"`
	Body       string   `json:"body" binding:"required"`
	PropertyID string   `json:"property_id"` // optional UUID; empty = all properties
	TenantIDs  []string `json:"tenant_ids"`  // optional; if set, fan out only to these tenants
	SendEmail  bool     `json:"send_email"`
}

// AnnouncementResponse is the DTO returned after creating an announcement.
type AnnouncementResponse struct {
	ID              string   `json:"id"`
	OwnerID         string   `json:"owner_id"`
	PropertyID      string   `json:"property_id,omitempty"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	SendEmail       bool     `json:"send_email"`
	CreatedAt       string   `json:"created_at"`
	NotifiedTenants []string `json:"notified_tenants"`
}
