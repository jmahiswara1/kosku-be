package dto

// CreateTicketRequest is the request body for POST /v1/tickets.
// Photo attachments are submitted as multipart form fields named "photos".
type CreateTicketRequest struct {
	Title       string `form:"title"       binding:"required"`
	Description string `form:"description" binding:"required"`
	Priority    string `form:"priority"`   // optional; defaults to "medium"
}

// UpdateTicketRequest is the request body for PUT /v1/tickets/:id.
type UpdateTicketRequest struct {
	Status     string `json:"status"     binding:"required,oneof=open in_progress resolved"`
	Priority   string `json:"priority"   binding:"required,oneof=low medium high urgent"`
	Resolution string `json:"resolution"`
}

// TicketAttachmentResponse is the response for a single ticket attachment.
type TicketAttachmentResponse struct {
	ID        string `json:"id"`
	TicketID  string `json:"ticket_id"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// TicketResponse is the standard ticket payload returned by ticket endpoints.
type TicketResponse struct {
	ID          string                     `json:"id"`
	TenantID    string                     `json:"tenant_id"`
	TenantName  string                     `json:"tenant_name,omitempty"`
	PropertyID  string                     `json:"property_id"`
	RoomID      string                     `json:"room_id,omitempty"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	Priority    string                     `json:"priority"`
	Status      string                     `json:"status"`
	Resolution  string                     `json:"resolution,omitempty"`
	Attachments []TicketAttachmentResponse `json:"attachments,omitempty"`
	CreatedAt   string                     `json:"created_at,omitempty"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
}
