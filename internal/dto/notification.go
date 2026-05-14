package dto

// NotificationResponse is the DTO returned for a single notification.
type NotificationResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	EntityID  string `json:"entity_id,omitempty"`
	IsRead    bool   `json:"is_read"`
	CreatedAt string `json:"created_at"`
}
