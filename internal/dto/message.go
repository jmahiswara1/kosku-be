package dto

// SendMessageRequest is the request body for POST /v1/messages.
type SendMessageRequest struct {
	ReceiverID string `json:"receiver_id" binding:"required,uuid"`
	PropertyID string `json:"property_id"` // optional
	Body       string `json:"body"         binding:"required"`
}

// MessageResponse is the standard message payload returned by messaging endpoints.
type MessageResponse struct {
	ID         string `json:"id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	PropertyID string `json:"property_id,omitempty"`
	Body       string `json:"body"`
	IsRead     bool   `json:"is_read"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// ConversationResponse represents a single conversation partner entry returned
// by GET /v1/messages.
type ConversationResponse struct {
	// PartnerID is the UUID of the other participant in the conversation.
	PartnerID   string `json:"partner_id"`
	PartnerName string `json:"partner_name,omitempty"`
	// LastMessage is the most recent message in the conversation.
	LastMessage MessageResponse `json:"last_message"`
}
