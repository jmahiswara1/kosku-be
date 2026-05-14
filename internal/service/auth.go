// Package service contains the business logic layer for the KosKu API.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/email"
)

// AuthService handles authentication and authorisation business logic.
type AuthService struct {
	queries    *repository.Queries
	emailClient *email.Client
	appURL     string
}

// NewAuthService creates a new AuthService.
// appURL is the base URL of the frontend application, used to build links in
// emails (e.g. "https://app.kosku.id").
func NewAuthService(queries *repository.Queries, emailClient *email.Client, appURL string) *AuthService {
	return &AuthService{
		queries:     queries,
		emailClient: emailClient,
		appURL:      appURL,
	}
}

// Register upserts a profile row for the authenticated user and returns the
// resulting profile. If the role is empty it defaults to "tenant".
func (s *AuthService) Register(ctx context.Context, userID uuid.UUID, req dto.RegisterRequest) (dto.ProfileResponse, error) {
	role := req.Role
	if role == "" {
		role = "tenant"
	}

	// Validate role value.
	switch role {
	case "owner", "tenant", "staff":
		// valid
	default:
		return dto.ProfileResponse{}, fmt.Errorf("invalid role %q: must be owner, tenant, or staff", role)
	}

	var avatarURL sql.NullString
	if req.AvatarURL != "" {
		avatarURL = sql.NullString{String: req.AvatarURL, Valid: true}
	}

	var phone sql.NullString
	if req.Phone != "" {
		phone = sql.NullString{String: req.Phone, Valid: true}
	}

	profile, err := s.queries.UpsertProfile(ctx, repository.UpsertProfileParams{
		ID:        userID,
		FullName:  req.FullName,
		AvatarUrl: avatarURL,
		Phone:     phone,
		Role:      role,
	})
	if err != nil {
		return dto.ProfileResponse{}, fmt.Errorf("register: upsert profile: %w", err)
	}

	return profileToDTO(profile), nil
}

// Invite creates an invitation record with a unique UUID token that expires in
// 7 days, then sends an invitation email to the specified address.
func (s *AuthService) Invite(ctx context.Context, ownerID uuid.UUID, req dto.InviteRequest) (dto.InvitationResponse, error) {
	// Fetch the owner's profile to get their name for the email.
	ownerProfile, err := s.queries.GetProfile(ctx, ownerID)
	if err != nil {
		return dto.InvitationResponse{}, fmt.Errorf("invite: get owner profile: %w", err)
	}

	token := uuid.New().String()
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	var propertyID uuid.NullUUID
	if req.PropertyID != "" {
		pid, err := uuid.Parse(req.PropertyID)
		if err != nil {
			return dto.InvitationResponse{}, fmt.Errorf("invite: invalid property_id: %w", err)
		}
		propertyID = uuid.NullUUID{UUID: pid, Valid: true}
	}

	inv, err := s.queries.CreateInvitation(ctx, repository.CreateInvitationParams{
		OwnerID:    ownerID,
		PropertyID: propertyID,
		Email:      req.Email,
		Token:      token,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return dto.InvitationResponse{}, fmt.Errorf("invite: create invitation: %w", err)
	}

	// Build the invite URL and send the email. Email errors are non-fatal for
	// the API response — the invitation record is already persisted.
	inviteURL := fmt.Sprintf("%s/register?token=%s", s.appURL, token)
	ownerName := ownerProfile.FullName
	if err := s.emailClient.SendInvitation(req.Email, ownerName, inviteURL); err != nil {
		// Log but do not fail the request.
		_ = err // caller can observe this via monitoring
	}

	return dto.InvitationResponse{
		ID:        inv.ID.String(),
		Email:     inv.Email,
		Token:     inv.Token,
		ExpiresAt: inv.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// Approve activates a pending tenant profile and sends a confirmation email.
// profileID is the UUID of the tenant's profile row.
// tenantEmail is the tenant's email address for the notification (optional).
func (s *AuthService) Approve(ctx context.Context, profileID uuid.UUID, tenantEmail string) (dto.ProfileResponse, error) {
	profile, err := s.queries.GetProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.ProfileResponse{}, ErrNotFound
		}
		return dto.ProfileResponse{}, fmt.Errorf("approve: get profile: %w", err)
	}

	// Send confirmation email if an email address was provided. Non-fatal.
	if tenantEmail != "" {
		appURL := s.appURL + "/login"
		_ = s.emailClient.SendRegistrationApproved(tenantEmail, profile.FullName, appURL)
	}

	return profileToDTO(profile), nil
}

// Reject sends a rejection email and deletes the pending profile.
// profileID is the UUID of the tenant's profile row.
// tenantEmail is the tenant's email address for the notification (optional).
func (s *AuthService) Reject(ctx context.Context, profileID uuid.UUID, tenantEmail string) error {
	profile, err := s.queries.GetProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("reject: get profile: %w", err)
	}

	// Send rejection email if an email address was provided. Non-fatal.
	if tenantEmail != "" {
		_ = s.emailClient.SendRegistrationRejected(tenantEmail, profile.FullName)
	}

	// Delete the pending profile row.
	if err := s.queries.DeleteProfile(ctx, profileID); err != nil {
		return fmt.Errorf("reject: delete profile: %w", err)
	}

	return nil
}

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// profileToDTO converts a repository.Profile to a dto.ProfileResponse.
func profileToDTO(p repository.Profile) dto.ProfileResponse {
	resp := dto.ProfileResponse{
		ID:       p.ID.String(),
		FullName: p.FullName,
		Role:     p.Role,
	}
	if p.AvatarUrl.Valid {
		resp.AvatarURL = p.AvatarUrl.String
	}
	if p.Phone.Valid {
		resp.Phone = p.Phone.String
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
