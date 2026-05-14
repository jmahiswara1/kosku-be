package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
)

// ErrRoomNotVacant is returned when a check-in is attempted on a non-vacant room.
var ErrRoomNotVacant = errors.New("room is not vacant")

// ErrTenantBlacklisted is returned when a check-in is attempted for a blacklisted tenant.
var ErrTenantBlacklisted = errors.New("tenant is blacklisted")

// ErrNoActiveContract is returned when a checkout is attempted but no active contract exists.
var ErrNoActiveContract = errors.New("no active contract found for tenant")

// ErrRefundExceedsDeposit is returned when the refund amount exceeds the deposit amount.
var ErrRefundExceedsDeposit = errors.New("refund amount exceeds deposit amount")

// TenantService handles business logic for tenant management.
type TenantService struct {
	queries *repository.Queries
}

// NewTenantService creates a new TenantService.
func NewTenantService(queries *repository.Queries) *TenantService {
	return &TenantService{queries: queries}
}

// ListTenants returns all tenants for the owner's properties with pagination.
// ownerID is used to find properties owned by the user; propertyID (optional)
// filters to a specific property.
func (s *TenantService) ListTenants(ctx context.Context, ownerID uuid.UUID, propertyID *uuid.UUID, limit, offset int) ([]dto.TenantResponse, int, error) {
	// If a specific property is requested, verify ownership.
	if propertyID != nil {
		prop, err := s.queries.GetProperty(ctx, *propertyID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, 0, ErrNotFound
			}
			return nil, 0, fmt.Errorf("list tenants: get property: %w", err)
		}
		if prop.OwnerID != ownerID {
			return nil, 0, ErrForbidden
		}

		rows, err := s.queries.ListTenants(ctx, uuid.NullUUID{UUID: *propertyID, Valid: true})
		if err != nil {
			return nil, 0, fmt.Errorf("list tenants: %w", err)
		}

		// Apply pagination in-memory.
		total := len(rows)
		if offset >= total {
			return []dto.TenantResponse{}, total, nil
		}
		end := offset + limit
		if end > total {
			end = total
		}
		rows = rows[offset:end]

		result := make([]dto.TenantResponse, 0, len(rows))
		for _, row := range rows {
			result = append(result, listTenantRowToDTO(row))
		}
		return result, total, nil
	}

	// No specific property — list all tenants across owner's properties.
	// Get all properties for the owner first.
	properties, err := s.queries.ListPropertiesByOwner(ctx, ownerID)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: list properties: %w", err)
	}

	var allTenants []dto.TenantResponse
	for _, prop := range properties {
		rows, err := s.queries.ListTenants(ctx, uuid.NullUUID{UUID: prop.ID, Valid: true})
		if err != nil {
			return nil, 0, fmt.Errorf("list tenants: list for property %s: %w", prop.ID, err)
		}
		for _, row := range rows {
			allTenants = append(allTenants, listTenantRowToDTO(row))
		}
	}

	total := len(allTenants)
	if offset >= total {
		return []dto.TenantResponse{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return allTenants[offset:end], total, nil
}

// GetTenant returns a single tenant profile, enforcing that the tenant belongs
// to one of the owner's properties.
func (s *TenantService) GetTenant(ctx context.Context, ownerID, tenantID uuid.UUID) (dto.TenantResponse, error) {
	row, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TenantResponse{}, ErrNotFound
		}
		return dto.TenantResponse{}, fmt.Errorf("get tenant: %w", err)
	}

	// Ownership check: verify the tenant's property belongs to the owner.
	if row.PropertyID.Valid {
		prop, err := s.queries.GetProperty(ctx, row.PropertyID.UUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.TenantResponse{}, ErrNotFound
			}
			return dto.TenantResponse{}, fmt.Errorf("get tenant: get property: %w", err)
		}
		if prop.OwnerID != ownerID {
			return dto.TenantResponse{}, ErrForbidden
		}
	}

	return getTenantRowToDTO(row), nil
}

// UpdateTenant updates a tenant's profile fields.
func (s *TenantService) UpdateTenant(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.UpdateTenantRequest) (dto.TenantResponse, error) {
	// Fetch existing tenant.
	existing, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TenantResponse{}, ErrNotFound
		}
		return dto.TenantResponse{}, fmt.Errorf("update tenant: get tenant: %w", err)
	}

	// Ownership check.
	if existing.PropertyID.Valid {
		prop, err := s.queries.GetProperty(ctx, existing.PropertyID.UUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.TenantResponse{}, ErrNotFound
			}
			return dto.TenantResponse{}, fmt.Errorf("update tenant: get property: %w", err)
		}
		if prop.OwnerID != ownerID {
			return dto.TenantResponse{}, ErrForbidden
		}
	}

	// Build update params — use existing values for fields not provided.
	ktpNumber := existing.KtpNumber.String
	if req.KTPNumber != nil {
		ktpNumber = *req.KTPNumber
	}
	occupation := existing.Occupation.String
	if req.Occupation != nil {
		occupation = *req.Occupation
	}
	emergencyName := existing.EmergencyName.String
	if req.EmergencyName != nil {
		emergencyName = *req.EmergencyName
	}
	emergencyPhone := existing.EmergencyPhone.String
	if req.EmergencyPhone != nil {
		emergencyPhone = *req.EmergencyPhone
	}

	updated, err := s.queries.UpdateTenant(ctx, repository.UpdateTenantParams{
		ID:             tenantID,
		PropertyID:     existing.PropertyID,
		RoomID:         existing.RoomID,
		KtpNumber:      nullableString(ktpNumber),
		KtpScanUrl:     existing.KtpScanUrl,
		Occupation:     nullableString(occupation),
		EmergencyName:  nullableString(emergencyName),
		EmergencyPhone: nullableString(emergencyPhone),
	})
	if err != nil {
		return dto.TenantResponse{}, fmt.Errorf("update tenant: %w", err)
	}

	// Also update profile if full_name or phone changed.
	if req.FullName != nil || req.Phone != nil {
		fullName := existing.FullName
		if req.FullName != nil {
			fullName = *req.FullName
		}
		phone := ""
		if existing.Phone.Valid {
			phone = existing.Phone.String
		}
		if req.Phone != nil {
			phone = *req.Phone
		}
		_, _ = s.queries.UpdateProfile(ctx, repository.UpdateProfileParams{
			ID:        tenantID,
			FullName:  fullName,
			AvatarUrl: existing.AvatarUrl,
			Phone:     nullableString(phone),
		})
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_tenant", "tenant", tenantID, map[string]string{"tenant_id": tenantID.String()}))

	// Re-fetch to get updated profile data.
	refreshed, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		// Fall back to returning what we have.
		return tenantToDTO(updated, existing.FullName, existing.Phone.String, existing.AvatarUrl.String), nil
	}
	return getTenantRowToDTO(refreshed), nil
}

// Checkin performs a tenant check-in: validates room is vacant, validates tenant
// is not blacklisted, creates a contract, updates room status to occupied, and
// writes an audit log.
func (s *TenantService) Checkin(ctx context.Context, ownerID uuid.UUID, req dto.CheckinRequest) (dto.CheckinResponse, error) {
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return dto.CheckinResponse{}, fmt.Errorf("checkin: invalid tenant_id: %w", err)
	}
	roomID, err := uuid.Parse(req.RoomID)
	if err != nil {
		return dto.CheckinResponse{}, fmt.Errorf("checkin: invalid room_id: %w", err)
	}
	propertyID, err := uuid.Parse(req.PropertyID)
	if err != nil {
		return dto.CheckinResponse{}, fmt.Errorf("checkin: invalid property_id: %w", err)
	}

	// Ownership check on property.
	prop, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.CheckinResponse{}, ErrNotFound
		}
		return dto.CheckinResponse{}, fmt.Errorf("checkin: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.CheckinResponse{}, ErrForbidden
	}

	// Validate room is vacant.
	room, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.CheckinResponse{}, ErrNotFound
		}
		return dto.CheckinResponse{}, fmt.Errorf("checkin: get room: %w", err)
	}
	if room.Status != "vacant" {
		return dto.CheckinResponse{}, ErrRoomNotVacant
	}

	// Validate tenant is not blacklisted.
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.CheckinResponse{}, ErrNotFound
		}
		return dto.CheckinResponse{}, fmt.Errorf("checkin: get tenant: %w", err)
	}
	if tenant.IsBlacklisted.Valid && tenant.IsBlacklisted.Bool {
		return dto.CheckinResponse{}, ErrTenantBlacklisted
	}

	// Create contract.
	monthlyPriceStr := strconv.FormatFloat(req.MonthlyPrice, 'f', 2, 64)
	var depositAmount sql.NullString
	if req.DepositAmount > 0 {
		depositAmount = sql.NullString{String: strconv.FormatFloat(req.DepositAmount, 'f', 2, 64), Valid: true}
	}

	contract, err := s.queries.CreateContract(ctx, repository.CreateContractParams{
		TenantID:        tenantID,
		RoomID:          roomID,
		PropertyID:      propertyID,
		StartDate:       req.StartDate,
		EndDate:         req.EndDate,
		MonthlyPrice:    monthlyPriceStr,
		DepositAmount:   depositAmount,
		DepositRefunded: sql.NullString{},
		Status:          "active",
		FileUrl:         sql.NullString{},
	})
	if err != nil {
		return dto.CheckinResponse{}, fmt.Errorf("checkin: create contract: %w", err)
	}

	// Update room status to occupied.
	_, err = s.queries.UpdateRoomStatus(ctx, repository.UpdateRoomStatusParams{
		ID:     roomID,
		Status: "occupied",
	})
	if err != nil {
		return dto.CheckinResponse{}, fmt.Errorf("checkin: update room status: %w", err)
	}

	// Update tenant's property_id and room_id.
	_, _ = s.queries.UpdateTenant(ctx, repository.UpdateTenantParams{
		ID:             tenantID,
		PropertyID:     uuid.NullUUID{UUID: propertyID, Valid: true},
		RoomID:         uuid.NullUUID{UUID: roomID, Valid: true},
		KtpNumber:      tenant.KtpNumber,
		KtpScanUrl:     tenant.KtpScanUrl,
		Occupation:     tenant.Occupation,
		EmergencyName:  tenant.EmergencyName,
		EmergencyPhone: tenant.EmergencyPhone,
	})

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "checkin", "contract", contract.ID, map[string]string{
		"tenant_id":   tenantID.String(),
		"room_id":     roomID.String(),
		"property_id": propertyID.String(),
		"contract_id": contract.ID.String(),
	}))

	// Re-fetch tenant to get updated data.
	updatedTenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		updatedTenant = tenant
	}

	return dto.CheckinResponse{
		Tenant:   getTenantRowToDTO(updatedTenant),
		Contract: contractToDTO(contract),
	}, nil
}

// ErrRefundExceedsDeposit is returned when the refund amount exceeds the deposit.
// Note: declared in tenant.go — do not redeclare here.

// Checkout performs a tenant check-out: finds the active contract, terminates it,
// updates room status to vacant, optionally records a deposit refund, and writes an audit log.
func (s *TenantService) Checkout(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.CheckoutRequest) (dto.ContractResponse, error) {
	// Fetch tenant to verify ownership.
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.ContractResponse{}, ErrNotFound
		}
		return dto.ContractResponse{}, fmt.Errorf("checkout: get tenant: %w", err)
	}

	if tenant.PropertyID.Valid {
		prop, err := s.queries.GetProperty(ctx, tenant.PropertyID.UUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.ContractResponse{}, ErrNotFound
			}
			return dto.ContractResponse{}, fmt.Errorf("checkout: get property: %w", err)
		}
		if prop.OwnerID != ownerID {
			return dto.ContractResponse{}, ErrForbidden
		}
	}

	// Find active contract.
	contract, err := s.queries.GetActiveContract(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.ContractResponse{}, ErrNoActiveContract
		}
		return dto.ContractResponse{}, fmt.Errorf("checkout: get active contract: %w", err)
	}

	// Validate deposit refund if provided.
	if req.RefundAmount > 0 {
		if contract.DepositAmount.Valid {
			depositAmount, parseErr := strconv.ParseFloat(contract.DepositAmount.String, 64)
			if parseErr == nil && req.RefundAmount > depositAmount {
				return dto.ContractResponse{}, ErrRefundExceedsDeposit
			}
		} else {
			// No deposit recorded — refund must be 0.
			return dto.ContractResponse{}, ErrRefundExceedsDeposit
		}
	}

	// Terminate the contract.
	terminated, err := s.queries.UpdateContractStatus(ctx, repository.UpdateContractStatusParams{
		ID:     contract.ID,
		Status: "terminated",
	})
	if err != nil {
		return dto.ContractResponse{}, fmt.Errorf("checkout: terminate contract: %w", err)
	}

	// Record deposit refund if provided.
	if req.RefundAmount > 0 {
		refundStr := strconv.FormatFloat(req.RefundAmount, 'f', 2, 64)
		updated, err := s.queries.UpdateContractDepositRefunded(ctx, repository.UpdateContractDepositRefundedParams{
			ID: contract.ID,
			DepositRefunded: sql.NullString{
				String: refundStr,
				Valid:  true,
			},
		})
		if err == nil {
			terminated = updated
		}
	}

	// Update room status to vacant.
	_, err = s.queries.UpdateRoomStatus(ctx, repository.UpdateRoomStatusParams{
		ID:     contract.RoomID,
		Status: "vacant",
	})
	if err != nil {
		return dto.ContractResponse{}, fmt.Errorf("checkout: update room status: %w", err)
	}

	// Clear tenant's room assignment.
	_, _ = s.queries.UpdateTenant(ctx, repository.UpdateTenantParams{
		ID:             tenantID,
		PropertyID:     tenant.PropertyID,
		RoomID:         uuid.NullUUID{Valid: false},
		KtpNumber:      tenant.KtpNumber,
		KtpScanUrl:     tenant.KtpScanUrl,
		Occupation:     tenant.Occupation,
		EmergencyName:  tenant.EmergencyName,
		EmergencyPhone: tenant.EmergencyPhone,
	})

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "checkout", "contract", contract.ID, map[string]string{
		"tenant_id":     tenantID.String(),
		"contract_id":   contract.ID.String(),
		"room_id":       contract.RoomID.String(),
		"refund_amount": strconv.FormatFloat(req.RefundAmount, 'f', 2, 64),
	}))

	return contractToDTO(terminated), nil
}

// Blacklist sets a tenant's is_blacklisted flag to true and stores the reason.
func (s *TenantService) Blacklist(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.BlacklistRequest) (dto.TenantResponse, error) {
	// Fetch tenant to verify ownership.
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TenantResponse{}, ErrNotFound
		}
		return dto.TenantResponse{}, fmt.Errorf("blacklist: get tenant: %w", err)
	}

	if tenant.PropertyID.Valid {
		prop, err := s.queries.GetProperty(ctx, tenant.PropertyID.UUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.TenantResponse{}, ErrNotFound
			}
			return dto.TenantResponse{}, fmt.Errorf("blacklist: get property: %w", err)
		}
		if prop.OwnerID != ownerID {
			return dto.TenantResponse{}, ErrForbidden
		}
	}

	updated, err := s.queries.BlacklistTenant(ctx, repository.BlacklistTenantParams{
		ID:              tenantID,
		BlacklistReason: nullableString(req.Reason),
	})
	if err != nil {
		return dto.TenantResponse{}, fmt.Errorf("blacklist: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "blacklist_tenant", "tenant", tenantID, map[string]string{
		"tenant_id": tenantID.String(),
		"reason":    req.Reason,
	}))

	return blacklistTenantToDTO(updated, tenant.FullName, tenant.Phone.String, tenant.AvatarUrl.String), nil
}

// listTenantRowToDTO converts a repository.ListTenantsRow to dto.TenantResponse.
func listTenantRowToDTO(row repository.ListTenantsRow) dto.TenantResponse {
	resp := dto.TenantResponse{
		ID:       row.ID.String(),
		FullName: row.FullName,
	}
	if row.PropertyID.Valid {
		resp.PropertyID = row.PropertyID.UUID.String()
	}
	if row.RoomID.Valid {
		resp.RoomID = row.RoomID.UUID.String()
	}
	if row.Phone.Valid {
		resp.Phone = row.Phone.String
	}
	if row.AvatarUrl.Valid {
		resp.AvatarURL = row.AvatarUrl.String
	}
	if row.KtpNumber.Valid {
		resp.KTPNumber = row.KtpNumber.String
	}
	if row.KtpScanUrl.Valid {
		resp.KTPScanURL = row.KtpScanUrl.String
	}
	if row.Occupation.Valid {
		resp.Occupation = row.Occupation.String
	}
	if row.EmergencyName.Valid {
		resp.EmergencyName = row.EmergencyName.String
	}
	if row.EmergencyPhone.Valid {
		resp.EmergencyPhone = row.EmergencyPhone.String
	}
	if row.IsBlacklisted.Valid {
		resp.IsBlacklisted = row.IsBlacklisted.Bool
	}
	if row.BlacklistReason.Valid {
		resp.BlacklistReason = row.BlacklistReason.String
	}
	if row.CreatedAt.Valid {
		resp.CreatedAt = row.CreatedAt.Time.Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		resp.UpdatedAt = row.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// getTenantRowToDTO converts a repository.GetTenantRow to dto.TenantResponse.
func getTenantRowToDTO(row repository.GetTenantRow) dto.TenantResponse {
	resp := dto.TenantResponse{
		ID:       row.ID.String(),
		FullName: row.FullName,
	}
	if row.PropertyID.Valid {
		resp.PropertyID = row.PropertyID.UUID.String()
	}
	if row.RoomID.Valid {
		resp.RoomID = row.RoomID.UUID.String()
	}
	if row.Phone.Valid {
		resp.Phone = row.Phone.String
	}
	if row.AvatarUrl.Valid {
		resp.AvatarURL = row.AvatarUrl.String
	}
	if row.KtpNumber.Valid {
		resp.KTPNumber = row.KtpNumber.String
	}
	if row.KtpScanUrl.Valid {
		resp.KTPScanURL = row.KtpScanUrl.String
	}
	if row.Occupation.Valid {
		resp.Occupation = row.Occupation.String
	}
	if row.EmergencyName.Valid {
		resp.EmergencyName = row.EmergencyName.String
	}
	if row.EmergencyPhone.Valid {
		resp.EmergencyPhone = row.EmergencyPhone.String
	}
	if row.IsBlacklisted.Valid {
		resp.IsBlacklisted = row.IsBlacklisted.Bool
	}
	if row.BlacklistReason.Valid {
		resp.BlacklistReason = row.BlacklistReason.String
	}
	if row.CreatedAt.Valid {
		resp.CreatedAt = row.CreatedAt.Time.Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		resp.UpdatedAt = row.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// tenantToDTO converts a repository.UpdateTenantRow to dto.TenantResponse.
func tenantToDTO(t repository.UpdateTenantRow, fullName, phone, avatarURL string) dto.TenantResponse {
	resp := dto.TenantResponse{
		ID:        t.ID.String(),
		FullName:  fullName,
		Phone:     phone,
		AvatarURL: avatarURL,
	}
	if t.PropertyID.Valid {
		resp.PropertyID = t.PropertyID.UUID.String()
	}
	if t.RoomID.Valid {
		resp.RoomID = t.RoomID.UUID.String()
	}
	if t.KtpNumber.Valid {
		resp.KTPNumber = t.KtpNumber.String
	}
	if t.KtpScanUrl.Valid {
		resp.KTPScanURL = t.KtpScanUrl.String
	}
	if t.Occupation.Valid {
		resp.Occupation = t.Occupation.String
	}
	if t.EmergencyName.Valid {
		resp.EmergencyName = t.EmergencyName.String
	}
	if t.EmergencyPhone.Valid {
		resp.EmergencyPhone = t.EmergencyPhone.String
	}
	if t.IsBlacklisted.Valid {
		resp.IsBlacklisted = t.IsBlacklisted.Bool
	}
	if t.BlacklistReason.Valid {
		resp.BlacklistReason = t.BlacklistReason.String
	}
	if t.CreatedAt.Valid {
		resp.CreatedAt = t.CreatedAt.Time.Format(time.RFC3339)
	}
	if t.UpdatedAt.Valid {
		resp.UpdatedAt = t.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// blacklistTenantToDTO converts a repository.BlacklistTenantRow to dto.TenantResponse.
func blacklistTenantToDTO(t repository.BlacklistTenantRow, fullName, phone, avatarURL string) dto.TenantResponse {
	resp := dto.TenantResponse{
		ID:        t.ID.String(),
		FullName:  fullName,
		Phone:     phone,
		AvatarURL: avatarURL,
	}
	if t.PropertyID.Valid {
		resp.PropertyID = t.PropertyID.UUID.String()
	}
	if t.RoomID.Valid {
		resp.RoomID = t.RoomID.UUID.String()
	}
	if t.KtpNumber.Valid {
		resp.KTPNumber = t.KtpNumber.String
	}
	if t.KtpScanUrl.Valid {
		resp.KTPScanURL = t.KtpScanUrl.String
	}
	if t.Occupation.Valid {
		resp.Occupation = t.Occupation.String
	}
	if t.EmergencyName.Valid {
		resp.EmergencyName = t.EmergencyName.String
	}
	if t.EmergencyPhone.Valid {
		resp.EmergencyPhone = t.EmergencyPhone.String
	}
	if t.IsBlacklisted.Valid {
		resp.IsBlacklisted = t.IsBlacklisted.Bool
	}
	if t.BlacklistReason.Valid {
		resp.BlacklistReason = t.BlacklistReason.String
	}
	if t.CreatedAt.Valid {
		resp.CreatedAt = t.CreatedAt.Time.Format(time.RFC3339)
	}
	if t.UpdatedAt.Valid {
		resp.UpdatedAt = t.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// contractToDTO converts a repository.Contract to dto.ContractResponse.
func contractToDTO(c repository.Contract) dto.ContractResponse {
	resp := dto.ContractResponse{
		ID:           c.ID.String(),
		TenantID:     c.TenantID.String(),
		RoomID:       c.RoomID.String(),
		PropertyID:   c.PropertyID.String(),
		MonthlyPrice: c.MonthlyPrice,
		Status:       c.Status,
	}
	resp.StartDate = c.StartDate.Format("2006-01-02")
	resp.EndDate = c.EndDate.Format("2006-01-02")
	if c.DepositAmount.Valid {
		resp.DepositAmount = c.DepositAmount.String
	}
	if c.DepositRefunded.Valid {
		resp.DepositRefunded = c.DepositRefunded.String
	}
	if c.FileUrl.Valid {
		resp.FileURL = c.FileUrl.String
	}
	if c.CreatedAt.Valid {
		resp.CreatedAt = c.CreatedAt.Time.Format(time.RFC3339)
	}
	if c.UpdatedAt.Valid {
		resp.UpdatedAt = c.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
