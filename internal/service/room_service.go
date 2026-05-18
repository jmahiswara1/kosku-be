package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/sqlc-dev/pqtype"
)

// RoomService handles business logic for room management.
type RoomService struct {
	queries *repository.Queries
	rawDB   *sql.DB
}

// NewRoomService creates a new RoomService.
func NewRoomService(queries *repository.Queries, db *sql.DB) *RoomService {
	return &RoomService{queries: queries, rawDB: db}
}

// ListRooms returns all non-archived rooms for a property, enforcing ownership.
func (s *RoomService) ListRooms(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error) {
	// Ownership check.
	prop, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("list rooms: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return nil, ErrForbidden
	}

	rows, err := s.queries.ListRoomsByProperty(ctx, propertyID)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}

	result := make([]dto.RoomResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, listRoomRowToDTO(row))
	}
	return result, nil
}

// CreateRoom inserts a new room into a property. If the room_type_name doesn't
// exist for the property, a new room_type row is created; otherwise the existing
// one is reused. Room number must be unique within the property.
func (s *RoomService) CreateRoom(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error) {
	// Ownership check.
	prop, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomResponse{}, ErrNotFound
		}
		return dto.RoomResponse{}, fmt.Errorf("create room: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.RoomResponse{}, ErrForbidden
	}

	// Resolve or create room type.
	roomType, err := s.queries.GetRoomTypeByName(ctx, repository.GetRoomTypeByNameParams{
		PropertyID: propertyID,
		Name:       req.RoomTypeName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Create a new room type.
			roomType, err = s.queries.CreateRoomType(ctx, repository.CreateRoomTypeParams{
				PropertyID:   propertyID,
				Name:         req.RoomTypeName,
				MonthlyPrice: req.MonthlyPrice,
			})
			if err != nil {
				return dto.RoomResponse{}, fmt.Errorf("create room: create room type: %w", err)
			}
		} else {
			return dto.RoomResponse{}, fmt.Errorf("create room: get room type: %w", err)
		}
	}

	// Build facilities JSON.
	facilities := req.Facilities
	if facilities == nil {
		facilities = []string{}
	}
	facilitiesJSON, err := json.Marshal(facilities)
	if err != nil {
		return dto.RoomResponse{}, fmt.Errorf("create room: marshal facilities: %w", err)
	}

	// Default status.
	status := req.Status
	if status == "" {
		status = "vacant"
	}

	room, err := s.queries.CreateRoom(ctx, repository.CreateRoomParams{
		PropertyID: propertyID,
		RoomTypeID: uuid.NullUUID{UUID: roomType.ID, Valid: true},
		Number:     req.Number,
		Floor:      nullableInt(req.Floor),
		Status:     status,
		GridX:      nullableInt(req.GridX),
		GridY:      nullableInt(req.GridY),
		Facilities: pqtype.NullRawMessage{RawMessage: facilitiesJSON, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return dto.RoomResponse{}, ErrDuplicateRoomNumber
		}
		return dto.RoomResponse{}, fmt.Errorf("create room: insert: %w", err)
	}

	return roomToDTO(room, &roomType), nil
}

// GetRoom returns a single room, enforcing that the authenticated owner owns
// the property that contains the room.
func (s *RoomService) GetRoom(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error) {
	row, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomResponse{}, ErrNotFound
		}
		return dto.RoomResponse{}, fmt.Errorf("get room: %w", err)
	}

	// Ownership check via property.
	prop, err := s.queries.GetProperty(ctx, row.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomResponse{}, ErrNotFound
		}
		return dto.RoomResponse{}, fmt.Errorf("get room: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.RoomResponse{}, ErrForbidden
	}

	return roomRowToDTO(row), nil
}

// UpdateRoom updates a room's fields, enforcing ownership.
func (s *RoomService) UpdateRoom(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error) {
	// Fetch existing room.
	existing, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomResponse{}, ErrNotFound
		}
		return dto.RoomResponse{}, fmt.Errorf("update room: get room: %w", err)
	}

	// Ownership check.
	prop, err := s.queries.GetProperty(ctx, existing.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomResponse{}, ErrNotFound
		}
		return dto.RoomResponse{}, fmt.Errorf("update room: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.RoomResponse{}, ErrForbidden
	}

	// Resolve or create room type.
	roomType, err := s.queries.GetRoomTypeByName(ctx, repository.GetRoomTypeByNameParams{
		PropertyID: existing.PropertyID,
		Name:       req.RoomTypeName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			roomType, err = s.queries.CreateRoomType(ctx, repository.CreateRoomTypeParams{
				PropertyID:   existing.PropertyID,
				Name:         req.RoomTypeName,
				MonthlyPrice: req.MonthlyPrice,
			})
			if err != nil {
				return dto.RoomResponse{}, fmt.Errorf("update room: create room type: %w", err)
			}
		} else {
			return dto.RoomResponse{}, fmt.Errorf("update room: get room type: %w", err)
		}
	}

	// Build facilities JSON.
	facilities := req.Facilities
	if facilities == nil {
		facilities = []string{}
	}
	facilitiesJSON, err := json.Marshal(facilities)
	if err != nil {
		return dto.RoomResponse{}, fmt.Errorf("update room: marshal facilities: %w", err)
	}

	status := req.Status
	if status == "" {
		status = existing.Status
	}

	updated, err := s.queries.UpdateRoom(ctx, repository.UpdateRoomParams{
		ID:         roomID,
		RoomTypeID: uuid.NullUUID{UUID: roomType.ID, Valid: true},
		Number:     req.Number,
		Floor:      nullableInt(req.Floor),
		Status:     status,
		GridX:      nullableInt(req.GridX),
		GridY:      nullableInt(req.GridY),
		Facilities: pqtype.NullRawMessage{RawMessage: facilitiesJSON, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return dto.RoomResponse{}, ErrDuplicateRoomNumber
		}
		return dto.RoomResponse{}, fmt.Errorf("update room: %w", err)
	}

	return roomUpdateToDTO(updated, &roomType), nil
}

// ArchiveRoom soft-archives a room, enforcing ownership.
func (s *RoomService) ArchiveRoom(ctx context.Context, ownerID, roomID uuid.UUID) error {
	existing, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("archive room: get room: %w", err)
	}

	prop, err := s.queries.GetProperty(ctx, existing.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("archive room: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return ErrForbidden
	}

	if err := s.queries.ArchiveRoom(ctx, roomID); err != nil {
		return fmt.Errorf("archive room: %w", err)
	}
	return nil
}

// UpdateLayout batch-updates grid_x/grid_y for all rooms in a property in a
// single transaction. Enforces ownership of the property.
func (s *RoomService) UpdateLayout(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error {
	// Ownership check.
	prop, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("update layout: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return ErrForbidden
	}

	// Execute all updates in a single transaction.
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update layout: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)
	for _, item := range req.Rooms {
		roomID, err := uuid.Parse(item.RoomID)
		if err != nil {
			return fmt.Errorf("update layout: invalid room_id %q: %w", item.RoomID, err)
		}
		if err := qtx.UpdateRoomLayout(ctx, repository.UpdateRoomLayoutParams{
			ID:    roomID,
			GridX: sql.NullInt32{Int32: int32(item.GridX), Valid: true}, //nolint:gosec // bounded grid coordinate
			GridY: sql.NullInt32{Int32: int32(item.GridY), Valid: true}, //nolint:gosec // bounded grid coordinate
		}); err != nil {
			return fmt.Errorf("update layout: update room %s: %w", item.RoomID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update layout: commit: %w", err)
	}
	return nil
}

// GetRoomHistory returns past contracts for a room ordered by start_date DESC,
// enforcing ownership.
func (s *RoomService) GetRoomHistory(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error) {
	// Fetch room to get property_id for ownership check.
	room, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get room history: get room: %w", err)
	}

	prop, err := s.queries.GetProperty(ctx, room.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get room history: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return nil, ErrForbidden
	}

	rows, err := s.queries.GetRoomHistory(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("get room history: %w", err)
	}

	result := make([]dto.RoomHistoryItem, 0, len(rows))
	for _, row := range rows {
		item := dto.RoomHistoryItem{
			ContractID:   row.ID.String(),
			TenantID:     row.TenantID.String(),
			TenantName:   row.TenantName,
			MonthlyPrice: row.MonthlyPrice,
			Status:       row.Status,
		}
		item.StartDate = formatDateField(row.StartDate)
		item.EndDate = formatDateField(row.EndDate)
		result = append(result, item)
	}
	return result, nil
}

// ErrDuplicateRoomNumber is returned when a room number already exists in the property.
var ErrDuplicateRoomNumber = errors.New("room number already exists in this property")

// nullableInt converts a *int to sql.NullInt32 for nullable DB columns.
func nullableInt(v *int) sql.NullInt32 {
	if v == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*v), Valid: true} //nolint:gosec // bounded grid/floor value
}

// isUniqueViolation checks if an error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return stringContains(msg, "unique") || stringContains(msg, "duplicate") || stringContains(msg, "23505")
}

func stringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// formatDateField converts an interface{} date value (from sqlc) to a date string.
func formatDateField(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02")
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// roomRowToDTO converts a repository.GetRoomRow to dto.RoomResponse.
func roomRowToDTO(row repository.GetRoomRow) dto.RoomResponse {
	resp := dto.RoomResponse{
		ID:         row.ID.String(),
		PropertyID: row.PropertyID.String(),
		Number:     row.Number,
		Status:     row.Status,
	}
	if row.Floor.Valid {
		v := row.Floor.Int32
		resp.Floor = &v
	}
	if row.GridX.Valid {
		v := row.GridX.Int32
		resp.GridX = &v
	}
	if row.GridY.Valid {
		v := row.GridY.Int32
		resp.GridY = &v
	}
	if row.Facilities.Valid && len(row.Facilities.RawMessage) > 0 {
		var fac []string
		if err := json.Unmarshal(row.Facilities.RawMessage, &fac); err == nil {
			resp.Facilities = fac
		}
	}
	if resp.Facilities == nil {
		resp.Facilities = []string{}
	}
	if row.RoomTypeID.Valid && row.RoomTypeName.Valid {
		price := ""
		if row.MonthlyPrice.Valid {
			price = row.MonthlyPrice.String
		}
		resp.RoomType = &dto.RoomTypeResponse{
			ID:           row.RoomTypeID.UUID.String(),
			Name:         row.RoomTypeName.String,
			MonthlyPrice: price,
		}
	}
	if row.CreatedAt.Valid {
		resp.CreatedAt = row.CreatedAt.Time.Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		resp.UpdatedAt = row.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// listRoomRowToDTO converts a repository.ListRoomsByPropertyRow to dto.RoomResponse.
func listRoomRowToDTO(row repository.ListRoomsByPropertyRow) dto.RoomResponse {
	resp := dto.RoomResponse{
		ID:         row.ID.String(),
		PropertyID: row.PropertyID.String(),
		Number:     row.Number,
		Status:     row.Status,
	}
	if row.Floor.Valid {
		v := row.Floor.Int32
		resp.Floor = &v
	}
	if row.GridX.Valid {
		v := row.GridX.Int32
		resp.GridX = &v
	}
	if row.GridY.Valid {
		v := row.GridY.Int32
		resp.GridY = &v
	}
	if row.Facilities.Valid && len(row.Facilities.RawMessage) > 0 {
		var fac []string
		if err := json.Unmarshal(row.Facilities.RawMessage, &fac); err == nil {
			resp.Facilities = fac
		}
	}
	if resp.Facilities == nil {
		resp.Facilities = []string{}
	}
	if row.RoomTypeID.Valid && row.RoomTypeName.Valid {
		price := ""
		if row.MonthlyPrice.Valid {
			price = row.MonthlyPrice.String
		}
		resp.RoomType = &dto.RoomTypeResponse{
			ID:           row.RoomTypeID.UUID.String(),
			Name:         row.RoomTypeName.String,
			MonthlyPrice: price,
		}
	}
	if row.CreatedAt.Valid {
		resp.CreatedAt = row.CreatedAt.Time.Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		resp.UpdatedAt = row.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// roomToDTO converts a repository.CreateRoomRow and optional RoomType to dto.RoomResponse.
func roomToDTO(room repository.CreateRoomRow, rt *repository.RoomType) dto.RoomResponse {
	resp := dto.RoomResponse{
		ID:         room.ID.String(),
		PropertyID: room.PropertyID.String(),
		Number:     room.Number,
		Status:     room.Status,
	}
	if room.Floor.Valid {
		v := room.Floor.Int32
		resp.Floor = &v
	}
	if room.GridX.Valid {
		v := room.GridX.Int32
		resp.GridX = &v
	}
	if room.GridY.Valid {
		v := room.GridY.Int32
		resp.GridY = &v
	}
	if room.Facilities.Valid && len(room.Facilities.RawMessage) > 0 {
		var fac []string
		if err := json.Unmarshal(room.Facilities.RawMessage, &fac); err == nil {
			resp.Facilities = fac
		}
	}
	if resp.Facilities == nil {
		resp.Facilities = []string{}
	}
	if rt != nil {
		resp.RoomType = &dto.RoomTypeResponse{
			ID:           rt.ID.String(),
			Name:         rt.Name,
			MonthlyPrice: rt.MonthlyPrice,
		}
	}
	if room.CreatedAt.Valid {
		resp.CreatedAt = room.CreatedAt.Time.Format(time.RFC3339)
	}
	if room.UpdatedAt.Valid {
		resp.UpdatedAt = room.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// roomUpdateToDTO converts a repository.UpdateRoomRow and optional RoomType to dto.RoomResponse.
func roomUpdateToDTO(room repository.UpdateRoomRow, rt *repository.RoomType) dto.RoomResponse {
	resp := dto.RoomResponse{
		ID:         room.ID.String(),
		PropertyID: room.PropertyID.String(),
		Number:     room.Number,
		Status:     room.Status,
	}
	if room.Floor.Valid {
		v := room.Floor.Int32
		resp.Floor = &v
	}
	if room.GridX.Valid {
		v := room.GridX.Int32
		resp.GridX = &v
	}
	if room.GridY.Valid {
		v := room.GridY.Int32
		resp.GridY = &v
	}
	if room.Facilities.Valid && len(room.Facilities.RawMessage) > 0 {
		var fac []string
		if err := json.Unmarshal(room.Facilities.RawMessage, &fac); err == nil {
			resp.Facilities = fac
		}
	}
	if resp.Facilities == nil {
		resp.Facilities = []string{}
	}
	if rt != nil {
		resp.RoomType = &dto.RoomTypeResponse{
			ID:           rt.ID.String(),
			Name:         rt.Name,
			MonthlyPrice: rt.MonthlyPrice,
		}
	}
	if room.CreatedAt.Valid {
		resp.CreatedAt = room.CreatedAt.Time.Format(time.RFC3339)
	}
	if room.UpdatedAt.Valid {
		resp.UpdatedAt = room.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
