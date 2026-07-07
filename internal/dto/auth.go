// Package dto defines Data Transfer Objects used for request/response
// serialization in the KosKu API handlers.
package dto

// RegisterRequest is the request body for POST /v1/auth/register.
// The Supabase JWT is read from the Authorization header by the auth
// middleware; this body carries the user's profile data.
type RegisterRequest struct {
	FullName  string `json:"full_name" binding:"required"`
	AvatarURL string `json:"avatar_url"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	// Role is the desired role for the user. Accepted values: owner, tenant, staff.
	// Defaults to "tenant" if omitted.
	Role string `json:"role"`
}

// InviteRequest is the request body for POST /v1/auth/invite.
type InviteRequest struct {
	Email      string `json:"email"       binding:"required,email"`
	PropertyID string `json:"property_id"`
}

// ApproveRequest is the optional request body for POST /v1/auth/approve/:id.
// Email is used to send the confirmation notification to the tenant.
type ApproveRequest struct {
	Email string `json:"email"`
}

// RejectRequest is the optional request body for POST /v1/auth/reject/:id.
// Email is used to send the rejection notification to the tenant.
type RejectRequest struct {
	Email string `json:"email"`
}

// ProfileResponse is the standard profile payload returned by auth endpoints.
type ProfileResponse struct {
	ID        string `json:"id"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// InvitationResponse is the payload returned after a successful invite.
type InvitationResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}
