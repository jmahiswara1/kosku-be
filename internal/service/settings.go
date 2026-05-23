// Package service contains the business logic layer for the KosKu API.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/email"
)

// SettingsService handles business logic for settings and staff management.
type SettingsService struct {
	queries     *repository.Queries
	emailClient *email.Client
	appURL      string
}

// NewSettingsService creates a new SettingsService.
func NewSettingsService(queries *repository.Queries, emailClient *email.Client, appURL string) *SettingsService {
	return &SettingsService{
		queries:     queries,
		emailClient: emailClient,
		appURL:      appURL,
	}
}

// GetSettings returns the combined settings (profile + billing config) for the owner's
// first property. If the owner has no properties, ErrNotFound is returned.
func (s *SettingsService) GetSettings(ctx context.Context, ownerID uuid.UUID) (dto.SettingsResponse, error) {
	row, err := s.queries.GetSettingsByOwner(ctx, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.SettingsResponse{}, ErrNotFound
		}
		return dto.SettingsResponse{}, fmt.Errorf("get settings: %w", err)
	}
	return settingsRowToDTO(row), nil
}

// UpdateProfileSettings updates the business profile fields of the owner's first property.
func (s *SettingsService) UpdateProfileSettings(ctx context.Context, ownerID uuid.UUID, req dto.UpdateProfileSettingsRequest) (dto.SettingsResponse, error) {
	// Get the owner's primary property.
	existing, err := s.queries.GetSettingsByOwner(ctx, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.SettingsResponse{}, ErrNotFound
		}
		return dto.SettingsResponse{}, fmt.Errorf("update profile settings: get property: %w", err)
	}

	updated, err := s.queries.UpdateProfileSettings(ctx, repository.UpdateProfileSettingsParams{
		ID:          existing.ID,
		Name:        req.Name,
		Address:     req.Address,
		City:        nullableStringSQL(req.City),
		LogoUrl:     nullableStringSQL(req.LogoURL),
		Phone:       nullableStringSQL(req.Phone),
		BankName:    nullableStringSQL(req.BankName),
		BankAccount: nullableStringSQL(req.BankAccount),
	})
	if err != nil {
		return dto.SettingsResponse{}, fmt.Errorf("update profile settings: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_profile_settings", "property", existing.ID, map[string]string{"property_id": existing.ID.String()}))

	return settingsUpdateProfileRowToDTO(updated), nil
}

// UpdateBillingSettings updates the billing configuration fields of the owner's first property.
func (s *SettingsService) UpdateBillingSettings(ctx context.Context, ownerID uuid.UUID, req dto.UpdateBillingSettingsRequest) (dto.SettingsResponse, error) {
	// Get the owner's primary property.
	existing, err := s.queries.GetSettingsByOwner(ctx, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.SettingsResponse{}, ErrNotFound
		}
		return dto.SettingsResponse{}, fmt.Errorf("update billing settings: get property: %w", err)
	}

	var penaltyAmount sql.NullString
	if req.PenaltyAmount > 0 {
		penaltyAmount = sql.NullString{
			String: strconv.FormatFloat(req.PenaltyAmount, 'f', 2, 64),
			Valid:  true,
		}
	}

	updated, err := s.queries.UpdateBillingSettings(ctx, repository.UpdateBillingSettingsParams{
		ID:              existing.ID,
		DueDateDay:      sql.NullInt32{Int32: int32(req.DueDateDay), Valid: true},      //nolint:gosec // bounded day 1-31
		GracePeriodDays: sql.NullInt32{Int32: int32(req.GracePeriodDays), Valid: true}, //nolint:gosec // bounded days value
		PenaltyType:     nullableStringSQL(req.PenaltyType),
		PenaltyAmount:   penaltyAmount,
	})
	if err != nil {
		return dto.SettingsResponse{}, fmt.Errorf("update billing settings: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_billing_settings", "property", existing.ID, map[string]string{"property_id": existing.ID.String()}))

	return settingsUpdateBillingRowToDTO(updated), nil
}

// ListStaff returns all staff members for the given owner.
func (s *SettingsService) ListStaff(ctx context.Context, ownerID uuid.UUID) ([]dto.StaffResponse, error) {
	rows, err := s.queries.ListStaffByOwner(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}

	result := make([]dto.StaffResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, staffRowToDTO(row))
	}
	return result, nil
}

// AddStaff sends an invitation email to the staff member and creates a staff_permissions row.
// The staff member must already have a profile (i.e., be registered). If not found, an
// invitation is sent and a placeholder staff_permissions row is created with the owner's ID.
func (s *SettingsService) AddStaff(ctx context.Context, ownerID uuid.UUID, req dto.AddStaffRequest) (dto.InvitationResponse, error) {
	// Fetch the owner's profile for the email.
	ownerProfile, err := s.queries.GetProfile(ctx, ownerID)
	if err != nil {
		return dto.InvitationResponse{}, fmt.Errorf("add staff: get owner profile: %w", err)
	}

	// Create an invitation record.
	token := uuid.New().String()
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	inv, err := s.queries.CreateInvitation(ctx, repository.CreateInvitationParams{
		OwnerID:    ownerID,
		PropertyID: uuid.NullUUID{},
		Email:      req.Email,
		Token:      token,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return dto.InvitationResponse{}, fmt.Errorf("add staff: create invitation: %w", err)
	}

	// Send invitation email — non-fatal.
	inviteURL := fmt.Sprintf("%s/register?token=%s&role=staff", s.appURL, token)
	_ = s.emailClient.SendStaffInvitation(req.Email, ownerProfile.FullName, inviteURL)

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "invite_staff", "invitation", inv.ID, map[string]string{"email": req.Email}))

	return dto.InvitationResponse{
		ID:        inv.ID.String(),
		Email:     inv.Email,
		Token:     inv.Token,
		ExpiresAt: inv.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// RemoveStaff removes a staff member's permissions for the given owner.
func (s *SettingsService) RemoveStaff(ctx context.Context, ownerID, staffID uuid.UUID) error {
	err := s.queries.DeleteStaffPermissions(ctx, repository.DeleteStaffPermissionsParams{
		StaffID: staffID,
		OwnerID: ownerID,
	})
	if err != nil {
		return fmt.Errorf("remove staff: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "remove_staff", "staff_permission", staffID, map[string]string{"staff_id": staffID.String()}))

	return nil
}

// ListAuditLogs returns paginated audit logs filterable by date range, actor, and action.
func (s *SettingsService) ListAuditLogs(ctx context.Context, _ uuid.UUID, actorIDStr, action, dateFrom, dateTo string, page, perPage int) ([]dto.AuditLogResponse, error) {
	// Build params with Column1=actorID, Column2=action, Column3=fromTime, Column4=toTime.
	var actorID uuid.UUID
	if actorIDStr != "" {
		if id, err := uuid.Parse(actorIDStr); err == nil {
			actorID = id
		}
	}

	var fromTime, toTime time.Time
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			fromTime = t
		}
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			toTime = t.Add(24*time.Hour - time.Second)
		}
	}

	params := repository.ListAuditLogsParams{
		Column1: actorID,
		Column2: action,
		Column3: fromTime,
		Column4: toTime,
		Limit:   int32(perPage),              //nolint:gosec // bounded pagination value
		Offset:  int32((page - 1) * perPage), //nolint:gosec // bounded pagination value
	}

	rows, err := s.queries.ListAuditLogs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}

	result := make([]dto.AuditLogResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, auditLogToDTO(row))
	}
	return result, nil
}

// ExportData generates a full data export for the owner and returns it as CSV bytes.
func (s *SettingsService) ExportData(ctx context.Context, ownerID uuid.UUID, format string) ([]byte, string, error) {
	// Fetch all data.
	properties, err := s.queries.ListPropertiesByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list properties: %w", err)
	}

	rooms, err := s.queries.ListRoomsByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list rooms: %w", err)
	}

	tenants, err := s.queries.ListTenantsByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list tenants: %w", err)
	}

	contracts, err := s.queries.ListContractsByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list contracts: %w", err)
	}

	bills, err := s.queries.ListBillsByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list bills: %w", err)
	}

	payments, err := s.queries.ListPaymentsByOwnerForExport(ctx, ownerID)
	if err != nil {
		return nil, "", fmt.Errorf("export: list payments: %w", err)
	}

	if format == "json" {
		data := map[string]interface{}{
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"properties":  properties,
			"rooms":       rooms,
			"tenants":     tenants,
			"contracts":   contracts,
			"bills":       bills,
			"payments":    payments,
		}
		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, "", fmt.Errorf("export: marshal json: %w", err)
		}
		return b, "application/json", nil
	}

	// Default: CSV format — generate a multi-section CSV.
	var buf []byte

	// Properties section.
	buf = append(buf, []byte("# PROPERTIES\n")...)
	buf = append(buf, []byte("id,name,address,city,phone,bank_name,bank_account,created_at\n")...)
	for _, p := range properties {
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(p.ID.String()),
			csvEscape(p.Name),
			csvEscape(p.Address),
			csvEscape(nullStringVal(p.City)),
			csvEscape(nullStringVal(p.Phone)),
			csvEscape(nullStringVal(p.BankName)),
			csvEscape(nullStringVal(p.BankAccount)),
			csvEscape(nullTimeVal(p.CreatedAt)),
		))...)
	}

	// Rooms section.
	buf = append(buf, []byte("\n# ROOMS\n")...)
	buf = append(buf, []byte("id,property_id,number,floor,status,created_at\n")...)
	for _, r := range rooms {
		floor := ""
		if r.Floor.Valid {
			floor = strconv.Itoa(int(r.Floor.Int32))
		}
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
			csvEscape(r.ID.String()),
			csvEscape(r.PropertyID.String()),
			csvEscape(r.Number),
			csvEscape(floor),
			csvEscape(r.Status),
			csvEscape(nullTimeVal(r.CreatedAt)),
		))...)
	}

	// Tenants section.
	buf = append(buf, []byte("\n# TENANTS\n")...)
	buf = append(buf, []byte("id,property_id,room_id,full_name,phone,ktp_number,occupation,is_blacklisted,created_at\n")...)
	for _, t := range tenants {
		propertyID := ""
		if t.PropertyID.Valid {
			propertyID = t.PropertyID.UUID.String()
		}
		roomID := ""
		if t.RoomID.Valid {
			roomID = t.RoomID.UUID.String()
		}
		isBlacklisted := "false"
		if t.IsBlacklisted.Valid && t.IsBlacklisted.Bool {
			isBlacklisted = "true"
		}
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(t.ID.String()),
			csvEscape(propertyID),
			csvEscape(roomID),
			csvEscape(t.FullName),
			csvEscape(nullStringVal(t.Phone)),
			csvEscape(nullStringVal(t.KtpNumber)),
			csvEscape(nullStringVal(t.Occupation)),
			csvEscape(isBlacklisted),
			csvEscape(nullTimeVal(t.CreatedAt)),
		))...)
	}

	// Contracts section.
	buf = append(buf, []byte("\n# CONTRACTS\n")...)
	buf = append(buf, []byte("id,tenant_id,room_id,property_id,start_date,end_date,monthly_price,deposit_amount,deposit_refunded,status,created_at\n")...)
	for _, c := range contracts {
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(c.ID.String()),
			csvEscape(c.TenantID.String()),
			csvEscape(c.RoomID.String()),
			csvEscape(c.PropertyID.String()),
			csvEscape(c.StartDate.Format("2006-01-02")),
			csvEscape(c.EndDate.Format("2006-01-02")),
			csvEscape(c.MonthlyPrice),
			csvEscape(nullStringVal(c.DepositAmount)),
			csvEscape(nullStringVal(c.DepositRefunded)),
			csvEscape(c.Status),
			csvEscape(nullTimeVal(c.CreatedAt)),
		))...)
	}

	// Bills section.
	buf = append(buf, []byte("\n# BILLS\n")...)
	buf = append(buf, []byte("id,tenant_id,property_id,room_id,period_month,period_year,base_amount,utility_amount,penalty_amount,total_amount,due_date,status,created_at\n")...)
	for _, b := range bills {
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%d,%d,%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(b.ID.String()),
			csvEscape(b.TenantID.String()),
			csvEscape(b.PropertyID.String()),
			csvEscape(b.RoomID.String()),
			b.PeriodMonth,
			b.PeriodYear,
			csvEscape(b.BaseAmount),
			csvEscape(nullStringVal(b.UtilityAmount)),
			csvEscape(nullStringVal(b.PenaltyAmount)),
			csvEscape(nullStringVal(b.TotalAmount)),
			csvEscape(b.DueDate.Format("2006-01-02")),
			csvEscape(b.Status),
			csvEscape(nullTimeVal(b.CreatedAt)),
		))...)
	}

	// Payments section.
	buf = append(buf, []byte("\n# PAYMENTS\n")...)
	buf = append(buf, []byte("id,bill_id,tenant_id,amount,status,confirmed_at,created_at\n")...)
	for _, p := range payments {
		confirmedAt := ""
		if p.ConfirmedAt.Valid {
			confirmedAt = p.ConfirmedAt.Time.Format(time.RFC3339)
		}
		buf = append(buf, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(p.ID.String()),
			csvEscape(p.BillID.String()),
			csvEscape(p.TenantID.String()),
			csvEscape(p.Amount),
			csvEscape(p.Status),
			csvEscape(confirmedAt),
			csvEscape(nullTimeVal(p.CreatedAt)),
		))...)
	}

	return buf, "text/csv", nil
}

//  helpers

// nullableStringSQL converts a string to sql.NullString.
func nullableStringSQL(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullStringVal extracts the string value from sql.NullString, returning "" if null.
func nullStringVal(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// nullTimeVal extracts the time value from sql.NullTime, returning "" if null.
func nullTimeVal(nt sql.NullTime) string {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return ""
}

// csvEscape wraps a value in quotes if it contains a comma, newline, or quote.
func csvEscape(s string) string {
	needsQuote := false
	for _, c := range s {
		if c == ',' || c == '\n' || c == '"' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return s
	}
	// Escape internal quotes by doubling them.
	escaped := ""
	for _, c := range s {
		if c == '"' {
			escaped += "\"\""
		} else {
			escaped += string(c)
		}
	}
	return `"` + escaped + `"`
}

// settingsRowToDTO converts a GetSettingsByOwnerRow to dto.SettingsResponse.
func settingsRowToDTO(row repository.GetSettingsByOwnerRow) dto.SettingsResponse {
	resp := dto.SettingsResponse{
		PropertyID:  row.ID.String(),
		Name:        row.Name,
		Address:     row.Address,
		City:        nullStringVal(row.City),
		LogoURL:     nullStringVal(row.LogoUrl),
		Phone:       nullStringVal(row.Phone),
		BankName:    nullStringVal(row.BankName),
		BankAccount: nullStringVal(row.BankAccount),
	}
	if row.DueDateDay.Valid {
		resp.DueDateDay = int(row.DueDateDay.Int32)
	}
	if row.GracePeriodDays.Valid {
		resp.GracePeriodDays = int(row.GracePeriodDays.Int32)
	}
	if row.PenaltyType.Valid {
		resp.PenaltyType = row.PenaltyType.String
	}
	if row.PenaltyAmount.Valid {
		v, _ := strconv.ParseFloat(row.PenaltyAmount.String, 64)
		resp.PenaltyAmount = v
	}
	return resp
}

// settingsUpdateProfileRowToDTO converts an UpdateProfileSettingsRow to dto.SettingsResponse.
func settingsUpdateProfileRowToDTO(row repository.UpdateProfileSettingsRow) dto.SettingsResponse {
	resp := dto.SettingsResponse{
		PropertyID:  row.ID.String(),
		Name:        row.Name,
		Address:     row.Address,
		City:        nullStringVal(row.City),
		LogoURL:     nullStringVal(row.LogoUrl),
		Phone:       nullStringVal(row.Phone),
		BankName:    nullStringVal(row.BankName),
		BankAccount: nullStringVal(row.BankAccount),
	}
	if row.DueDateDay.Valid {
		resp.DueDateDay = int(row.DueDateDay.Int32)
	}
	if row.GracePeriodDays.Valid {
		resp.GracePeriodDays = int(row.GracePeriodDays.Int32)
	}
	if row.PenaltyType.Valid {
		resp.PenaltyType = row.PenaltyType.String
	}
	if row.PenaltyAmount.Valid {
		v, _ := strconv.ParseFloat(row.PenaltyAmount.String, 64)
		resp.PenaltyAmount = v
	}
	return resp
}

// settingsUpdateBillingRowToDTO converts an UpdateBillingSettingsRow to dto.SettingsResponse.
func settingsUpdateBillingRowToDTO(row repository.UpdateBillingSettingsRow) dto.SettingsResponse {
	resp := dto.SettingsResponse{
		PropertyID:  row.ID.String(),
		Name:        row.Name,
		Address:     row.Address,
		City:        nullStringVal(row.City),
		LogoURL:     nullStringVal(row.LogoUrl),
		Phone:       nullStringVal(row.Phone),
		BankName:    nullStringVal(row.BankName),
		BankAccount: nullStringVal(row.BankAccount),
	}
	if row.DueDateDay.Valid {
		resp.DueDateDay = int(row.DueDateDay.Int32)
	}
	if row.GracePeriodDays.Valid {
		resp.GracePeriodDays = int(row.GracePeriodDays.Int32)
	}
	if row.PenaltyType.Valid {
		resp.PenaltyType = row.PenaltyType.String
	}
	if row.PenaltyAmount.Valid {
		v, _ := strconv.ParseFloat(row.PenaltyAmount.String, 64)
		resp.PenaltyAmount = v
	}
	return resp
}

// staffRowToDTO converts a ListStaffByOwnerRow to dto.StaffResponse.
func staffRowToDTO(row repository.ListStaffByOwnerRow) dto.StaffResponse {
	var modules []string
	_ = json.Unmarshal(row.Modules, &modules)
	if modules == nil {
		modules = []string{}
	}

	resp := dto.StaffResponse{
		ID:       row.ID.String(),
		StaffID:  row.StaffID.String(),
		OwnerID:  row.OwnerID.String(),
		FullName: row.FullName,
		Modules:  modules,
	}
	if row.AvatarUrl.Valid {
		resp.AvatarURL = row.AvatarUrl.String
	}
	if row.Phone.Valid {
		resp.Phone = row.Phone.String
	}
	if row.CreatedAt.Valid {
		resp.CreatedAt = row.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// auditLogToDTO converts a repository.AuditLog to dto.AuditLogResponse.
func auditLogToDTO(log repository.AuditLog) dto.AuditLogResponse {
	resp := dto.AuditLogResponse{
		ID:         log.ID.String(),
		ActorID:    log.ActorID.String(),
		Action:     log.Action,
		EntityType: log.EntityType,
		Metadata:   log.Metadata,
	}
	if log.EntityID.Valid {
		resp.EntityID = log.EntityID.UUID.String()
	}
	if log.CreatedAt.Valid {
		resp.CreatedAt = log.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
