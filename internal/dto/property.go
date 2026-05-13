package dto

// CreatePropertyRequest is the request body for POST /v1/properties.
type CreatePropertyRequest struct {
	Name        string `json:"name"    binding:"required"`
	Address     string `json:"address" binding:"required"`
	Phone       string `json:"phone"   binding:"required"`
	City        string `json:"city"`
	LogoURL     string `json:"logo_url"`
	BankName    string `json:"bank_name"`
	BankAccount string `json:"bank_account"`
}

// UpdatePropertyRequest is the request body for PUT /v1/properties/:id.
type UpdatePropertyRequest struct {
	Name        string `json:"name"    binding:"required"`
	Address     string `json:"address" binding:"required"`
	Phone       string `json:"phone"   binding:"required"`
	City        string `json:"city"`
	LogoURL     string `json:"logo_url"`
	BankName    string `json:"bank_name"`
	BankAccount string `json:"bank_account"`
}

// PropertyStats holds aggregated room statistics for a property.
type PropertyStats struct {
	TotalRooms    int64   `json:"total_rooms"`
	OccupiedRooms int64   `json:"occupied_rooms"`
	OccupancyRate float64 `json:"occupancy_rate"` // percentage 0–100
}

// PropertyResponse is the standard property payload returned by property endpoints.
type PropertyResponse struct {
	ID          string        `json:"id"`
	OwnerID     string        `json:"owner_id"`
	Name        string        `json:"name"`
	Address     string        `json:"address"`
	City        string        `json:"city,omitempty"`
	LogoURL     string        `json:"logo_url,omitempty"`
	Phone       string        `json:"phone,omitempty"`
	BankName    string        `json:"bank_name,omitempty"`
	BankAccount string        `json:"bank_account,omitempty"`
	Stats       PropertyStats `json:"stats"`
	CreatedAt   string        `json:"created_at,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
}
