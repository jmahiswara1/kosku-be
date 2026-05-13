package dto

// RoomPhotoResponse is the standard room photo payload returned by photo endpoints.
type RoomPhotoResponse struct {
	ID        string `json:"id"`
	RoomID    string `json:"room_id"`
	URL       string `json:"url"`
	OrderIdx  int    `json:"order_idx"`
	CreatedAt string `json:"created_at,omitempty"`
}
