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
)

// PropertyService handles business logic for property management.
type PropertyService struct {
	queries *repository.Queries
}

// NewPropertyService creates a new PropertyService.
func NewPropertyService(queries *repository.Queries) *PropertyService {
	return &PropertyService{queries: queries}
}

// ListProperties returns all non-archived properties for the given owner,
// including summary stats (total rooms, occupied rooms, occupancy rate).
func (s *PropertyService) ListProperties(ctx context.Context, ownerID uuid.UUID) ([]dto.PropertyResponse, error) {
	rows, err := s.queries.ListPropertiesWithStatsByOwner(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}

	result := make([]dto.PropertyResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, propertyWithStatsToDTO(row))
	}
	return result, nil
}

// CreateProperty inserts a new property row and writes an audit log entry.
func (s *PropertyService) CreateProperty(ctx context.Context, ownerID uuid.UUID, req dto.CreatePropertyRequest) (dto.PropertyResponse, error) {
	prop, err := s.queries.CreateProperty(ctx, repository.CreatePropertyParams{
		OwnerID:     ownerID,
		Name:        req.Name,
		Address:     req.Address,
		City:        nullableString(req.City),
		LogoUrl:     nullableString(req.LogoURL),
		Phone:       nullableString(req.Phone),
		BankName:    nullableString(req.BankName),
		BankAccount: nullableString(req.BankAccount),
	})
	if err != nil {
		return dto.PropertyResponse{}, fmt.Errorf("create property: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "create_property", "property", prop.ID, map[string]string{"name": prop.Name}))

	return propertyCreateRowToDTO(prop), nil
}
func (s *PropertyService) GetProperty(ctx context.Context, ownerID, propertyID uuid.UUID) (dto.PropertyResponse, error) {
	row, err := s.queries.GetPropertyWithStats(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.PropertyResponse{}, ErrNotFound
		}
		return dto.PropertyResponse{}, fmt.Errorf("get property: %w", err)
	}

	if row.OwnerID != ownerID {
		return dto.PropertyResponse{}, ErrForbidden
	}

	return propertyGetWithStatsToDTO(row), nil
}

// UpdateProperty updates a property's fields and writes an audit log entry.
// It enforces owner ownership before updating.
func (s *PropertyService) UpdateProperty(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (dto.PropertyResponse, error) {
	// Ownership check.
	existing, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.PropertyResponse{}, ErrNotFound
		}
		return dto.PropertyResponse{}, fmt.Errorf("update property: get existing: %w", err)
	}
	if existing.OwnerID != ownerID {
		return dto.PropertyResponse{}, ErrForbidden
	}

	updated, err := s.queries.UpdateProperty(ctx, repository.UpdatePropertyParams{
		ID:          propertyID,
		Name:        req.Name,
		Address:     req.Address,
		City:        nullableString(req.City),
		LogoUrl:     nullableString(req.LogoURL),
		Phone:       nullableString(req.Phone),
		BankName:    nullableString(req.BankName),
		BankAccount: nullableString(req.BankAccount),
	})
	if err != nil {
		return dto.PropertyResponse{}, fmt.Errorf("update property: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_property", "property", updated.ID, map[string]string{"name": updated.Name}))

	return propertyUpdateRowToDTO(updated), nil
}

// ArchiveProperty soft-archives a property and cascade-archives its rooms and
// tenants. Requires explicit confirmation from the caller.
// It enforces owner ownership before archiving.
func (s *PropertyService) ArchiveProperty(ctx context.Context, ownerID, propertyID uuid.UUID) error {
	// Ownership check.
	existing, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("archive property: get existing: %w", err)
	}
	if existing.OwnerID != ownerID {
		return ErrForbidden
	}

	// Cascade-archive tenants first (FK dependency order).
	if err := s.queries.ArchiveTenantsByProperty(ctx, uuid.NullUUID{UUID: propertyID, Valid: true}); err != nil {
		return fmt.Errorf("archive property: archive tenants: %w", err)
	}

	// Cascade-archive rooms.
	if err := s.queries.ArchiveRoomsByProperty(ctx, propertyID); err != nil {
		return fmt.Errorf("archive property: archive rooms: %w", err)
	}

	// Archive the property itself.
	if err := s.queries.ArchiveProperty(ctx, propertyID); err != nil {
		return fmt.Errorf("archive property: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "archive_property", "property", propertyID, map[string]string{"name": existing.Name}))

	return nil
}

// ErrForbidden is returned when the caller does not own the requested resource.
var ErrForbidden = errors.New("forbidden")

// nullableString converts an empty string to sql.NullString (for nullable DB columns).
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// propertyCreateRowToDTO converts a repository.CreatePropertyRow to a dto.PropertyResponse.
func propertyCreateRowToDTO(p repository.CreatePropertyRow) dto.PropertyResponse {
	resp := dto.PropertyResponse{
		ID:      p.ID.String(),
		OwnerID: p.OwnerID.String(),
		Name:    p.Name,
		Address: p.Address,
		Stats:   buildStats(0, 0),
	}
	if p.City.Valid {
		resp.City = p.City.String
	}
	if p.LogoUrl.Valid {
		resp.LogoURL = p.LogoUrl.String
	}
	if p.Phone.Valid {
		resp.Phone = p.Phone.String
	}
	if p.BankName.Valid {
		resp.BankName = p.BankName.String
	}
	if p.BankAccount.Valid {
		resp.BankAccount = p.BankAccount.String
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// propertyUpdateRowToDTO converts a repository.UpdatePropertyRow to a dto.PropertyResponse.
func propertyUpdateRowToDTO(p repository.UpdatePropertyRow) dto.PropertyResponse {
	resp := dto.PropertyResponse{
		ID:      p.ID.String(),
		OwnerID: p.OwnerID.String(),
		Name:    p.Name,
		Address: p.Address,
		Stats:   buildStats(0, 0),
	}
	if p.City.Valid {
		resp.City = p.City.String
	}
	if p.LogoUrl.Valid {
		resp.LogoURL = p.LogoUrl.String
	}
	if p.Phone.Valid {
		resp.Phone = p.Phone.String
	}
	if p.BankName.Valid {
		resp.BankName = p.BankName.String
	}
	if p.BankAccount.Valid {
		resp.BankAccount = p.BankAccount.String
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// propertyWithStatsToDTO converts a repository.ListPropertiesWithStatsByOwnerRow to a dto.PropertyResponse.
func propertyWithStatsToDTO(p repository.ListPropertiesWithStatsByOwnerRow) dto.PropertyResponse {
	resp := dto.PropertyResponse{
		ID:      p.ID.String(),
		OwnerID: p.OwnerID.String(),
		Name:    p.Name,
		Address: p.Address,
		Stats:   buildStats(p.TotalRooms, p.OccupiedRooms),
	}
	if p.City.Valid {
		resp.City = p.City.String
	}
	if p.LogoUrl.Valid {
		resp.LogoURL = p.LogoUrl.String
	}
	if p.Phone.Valid {
		resp.Phone = p.Phone.String
	}
	if p.BankName.Valid {
		resp.BankName = p.BankName.String
	}
	if p.BankAccount.Valid {
		resp.BankAccount = p.BankAccount.String
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// propertyGetWithStatsToDTO converts a repository.GetPropertyWithStatsRow to a dto.PropertyResponse.
func propertyGetWithStatsToDTO(p repository.GetPropertyWithStatsRow) dto.PropertyResponse {
	resp := dto.PropertyResponse{
		ID:      p.ID.String(),
		OwnerID: p.OwnerID.String(),
		Name:    p.Name,
		Address: p.Address,
		Stats:   buildStats(p.TotalRooms, p.OccupiedRooms),
	}
	if p.City.Valid {
		resp.City = p.City.String
	}
	if p.LogoUrl.Valid {
		resp.LogoURL = p.LogoUrl.String
	}
	if p.Phone.Valid {
		resp.Phone = p.Phone.String
	}
	if p.BankName.Valid {
		resp.BankName = p.BankName.String
	}
	if p.BankAccount.Valid {
		resp.BankAccount = p.BankAccount.String
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// buildStats computes the PropertyStats from raw counts.
func buildStats(totalRooms, occupiedRooms int64) dto.PropertyStats {
	var rate float64
	if totalRooms > 0 {
		rate = float64(occupiedRooms) / float64(totalRooms) * 100
	}
	return dto.PropertyStats{
		TotalRooms:    totalRooms,
		OccupiedRooms: occupiedRooms,
		OccupancyRate: rate,
	}
}
