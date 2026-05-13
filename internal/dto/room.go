package dto

// CreateRoomRequest is the request body for POST /v1/properties/:id/rooms.
type CreateRoomRequest struct {
	Number       string   `json:"number"        binding:"required"`
	RoomTypeName string   `json:"room_type_name" binding:"required"`
	MonthlyPrice string   `json:"monthly_price"  binding:"required"`
	Floor        *int     `json:"floor"`
	Status       string   `json:"status"`
	GridX        *int     `json:"grid_x"`
	GridY        *int     `json:"grid_y"`
	Facilities   []string `json:"facilities"`
}

// UpdateRoomRequest is the request body for PUT /v1/rooms/:id.
type UpdateRoomRequest struct {
	Number       string   `json:"number"        binding:"required"`
	RoomTypeName string   `json:"room_type_name" binding:"required"`
	MonthlyPrice string   `json:"monthly_price"  binding:"required"`
	Floor        *int     `json:"floor"`
	Status       string   `json:"status"`
	GridX        *int     `json:"grid_x"`
	GridY        *int     `json:"grid_y"`
	Facilities   []string `json:"facilities"`
}

// RoomTypeResponse is the room type sub-object returned in room responses.
type RoomTypeResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MonthlyPrice string `json:"monthly_price"`
}

// RoomResponse is the standard room payload returned by room endpoints.
type RoomResponse struct {
	ID         string            `json:"id"`
	PropertyID string            `json:"property_id"`
	Number     string            `json:"number"`
	Floor      *int32            `json:"floor,omitempty"`
	Status     string            `json:"status"`
	GridX      *int32            `json:"grid_x,omitempty"`
	GridY      *int32            `json:"grid_y,omitempty"`
	Facilities []string          `json:"facilities"`
	RoomType   *RoomTypeResponse `json:"room_type,omitempty"`
	CreatedAt  string            `json:"created_at,omitempty"`
	UpdatedAt  string            `json:"updated_at,omitempty"`
}

// LayoutItem represents a single room's grid position in a layout update.
type LayoutItem struct {
	RoomID string `json:"room_id" binding:"required"`
	GridX  int    `json:"grid_x"`
	GridY  int    `json:"grid_y"`
}

// UpdateLayoutRequest is the request body for PUT /v1/properties/:id/layout.
type UpdateLayoutRequest struct {
	Rooms []LayoutItem `json:"rooms" binding:"required"`
}

// RoomHistoryItem represents a single past contract entry in room history.
type RoomHistoryItem struct {
	ContractID   string `json:"contract_id"`
	TenantID     string `json:"tenant_id"`
	TenantName   string `json:"tenant_name"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	MonthlyPrice string `json:"monthly_price"`
	Status       string `json:"status"`
}
