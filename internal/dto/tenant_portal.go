package dto

// TenantRoomResponse is the response for GET /v1/me/room.
type TenantRoomResponse struct {
	RoomID         string            `json:"room_id"`
	RoomNumber     string            `json:"room_number"`
	RoomType       string            `json:"room_type,omitempty"`
	Floor          int               `json:"floor,omitempty"`
	Status         string            `json:"status"`
	MonthlyPrice   string            `json:"monthly_price,omitempty"`
	PropertyID     string            `json:"property_id,omitempty"`
	ActiveContract *ContractResponse `json:"active_contract,omitempty"`
}

// ContractRenewalRequest is the request body for POST /v1/me/contracts/renew.
type ContractRenewalRequest struct {
	RequestedEndDate string `json:"requested_end_date"`
	Notes            string `json:"notes"`
}
