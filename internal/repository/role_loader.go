package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// GetProfileRole returns the role string for the given user ID.
// It returns an empty string and a non-nil error if the profile does not exist.
// This method satisfies the middleware.RoleLoader interface.
func (q *Queries) GetProfileRole(ctx context.Context, id uuid.UUID) (string, error) {
	profile, err := q.GetProfile(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Profile not yet created (first-time registration) — not an error
			// from the caller's perspective; the middleware will fall back to JWT claims.
			return "", nil
		}
		return "", err
	}
	return profile.Role, nil
}
