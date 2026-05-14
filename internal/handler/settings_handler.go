package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// SettingsServicer is the interface that SettingsHandler depends on.
// It is satisfied by *service.SettingsService and can be implemented by test mocks.
type SettingsServicer interface {
	GetSettings(ctx context.Context, ownerID uuid.UUID) (dto.SettingsResponse, error)
	UpdateProfileSettings(ctx context.Context, ownerID uuid.UUID, req dto.UpdateProfileSettingsRequest) (dto.SettingsResponse, error)
	UpdateBillingSettings(ctx context.Context, ownerID uuid.UUID, req dto.UpdateBillingSettingsRequest) (dto.SettingsResponse, error)
	ListStaff(ctx context.Context, ownerID uuid.UUID) ([]dto.StaffResponse, error)
	AddStaff(ctx context.Context, ownerID uuid.UUID, req dto.AddStaffRequest) (dto.InvitationResponse, error)
	RemoveStaff(ctx context.Context, ownerID, staffID uuid.UUID) error
	ListAuditLogs(ctx context.Context, ownerID uuid.UUID, actorIDStr, action, dateFrom, dateTo string, page, perPage int) ([]dto.AuditLogResponse, error)
	ExportData(ctx context.Context, ownerID uuid.UUID, format string) ([]byte, string, error)
}

// Ensure *service.SettingsService satisfies SettingsServicer at compile time.
var _ SettingsServicer = (*service.SettingsService)(nil)

// SettingsHandler holds the dependencies for settings-related HTTP handlers.
type SettingsHandler struct {
	svc SettingsServicer
}

// NewSettingsHandler creates a new SettingsHandler backed by a *service.SettingsService.
func NewSettingsHandler(svc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

// NewSettingsHandlerWithService creates a new SettingsHandler with any SettingsServicer.
// Intended for use in tests.
func NewSettingsHandlerWithService(svc SettingsServicer) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

// GetSettings handles GET /v1/settings.
// Returns the combined settings (business profile + billing config) for the owner.
//
//	@Summary		Get owner settings
//	@Description	Returns the combined business profile and billing configuration for the owner's primary property.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/settings [get]
//	@Security		BearerAuth
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	settings, err := h.svc.GetSettings(c.Request.Context(), ownerID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "No property found for this owner"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_SETTINGS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(settings))
}

// UpdateProfileSettings handles PUT /v1/settings/profile.
// Updates the business profile fields of the owner's primary property.
//
//	@Summary		Update business profile settings
//	@Description	Updates the kos name, address, phone, logo, and bank account details.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.UpdateProfileSettingsRequest	true	"Profile settings"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/settings/profile [put]
//	@Security		BearerAuth
func (h *SettingsHandler) UpdateProfileSettings(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.UpdateProfileSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	settings, err := h.svc.UpdateProfileSettings(c.Request.Context(), ownerID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "No property found for this owner"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_PROFILE_SETTINGS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(settings))
}

// UpdateBillingSettings handles PUT /v1/settings/billing.
// Updates the billing configuration (due date, grace period, penalty) of the owner's primary property.
//
//	@Summary		Update billing configuration
//	@Description	Updates the monthly due date, grace period, and late payment penalty settings.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.UpdateBillingSettingsRequest	true	"Billing settings"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/settings/billing [put]
//	@Security		BearerAuth
func (h *SettingsHandler) UpdateBillingSettings(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.UpdateBillingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	settings, err := h.svc.UpdateBillingSettings(c.Request.Context(), ownerID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "No property found for this owner"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_BILLING_SETTINGS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(settings))
}

// ListStaff handles GET /v1/settings/staff.
// Returns all staff members for the authenticated owner.
//
//	@Summary		List staff members
//	@Description	Returns all staff members and their module permissions for the authenticated owner.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/settings/staff [get]
//	@Security		BearerAuth
func (h *SettingsHandler) ListStaff(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	staff, err := h.svc.ListStaff(c.Request.Context(), ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_STAFF_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    staff,
		"meta":    gin.H{"total": len(staff)},
	})
}

// AddStaff handles POST /v1/settings/staff.
// Sends an invitation email to the staff member and creates a staff_permissions row.
//
//	@Summary		Add staff member
//	@Description	Sends an invitation email to the specified email address and creates a staff permissions record.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.AddStaffRequest	true	"Staff invitation data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Router			/settings/staff [post]
//	@Security		BearerAuth
func (h *SettingsHandler) AddStaff(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.AddStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	inv, err := h.svc.AddStaff(c.Request.Context(), ownerID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("ADD_STAFF_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(inv))
}

// RemoveStaff handles DELETE /v1/settings/staff/:id.
// Removes a staff member's permissions for the authenticated owner.
//
//	@Summary		Remove staff member
//	@Description	Removes a staff member's access permissions for the authenticated owner.
//	@Tags			settings
//	@Produce		json
//	@Param			id	path		string	true	"Staff profile UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Router			/settings/staff/{id} [delete]
//	@Security		BearerAuth
func (h *SettingsHandler) RemoveStaff(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	staffID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid staff ID"))
		return
	}

	if err := h.svc.RemoveStaff(c.Request.Context(), ownerID, staffID); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("REMOVE_STAFF_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Staff member removed successfully"},
	})
}

// ListAuditLogs handles GET /v1/audit-logs.
// Returns paginated audit logs filterable by date range, actor, and action type.
//
//	@Summary		List audit logs
//	@Description	Returns paginated audit logs. Filterable by date range, actor ID, and action type.
//	@Tags			audit
//	@Produce		json
//	@Param			actor_id	query	string	false	"Filter by actor UUID"
//	@Param			action		query	string	false	"Filter by action type"
//	@Param			date_from	query	string	false	"Filter from date (YYYY-MM-DD)"
//	@Param			date_to		query	string	false	"Filter to date (YYYY-MM-DD)"
//	@Param			page		query	int		false	"Page number (default 1)"
//	@Param			per_page	query	int		false	"Items per page (default 20)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/audit-logs [get]
//	@Security		BearerAuth
func (h *SettingsHandler) ListAuditLogs(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	page := 1
	perPage := 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := c.Query("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}

	logs, err := h.svc.ListAuditLogs(
		c.Request.Context(),
		ownerID,
		c.Query("actor_id"),
		c.Query("action"),
		c.Query("date_from"),
		c.Query("date_to"),
		page,
		perPage,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_AUDIT_LOGS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
		"meta": gin.H{
			"page":     page,
			"per_page": perPage,
		},
	})
}

// ExportData handles GET /v1/export.
// Generates a CSV or JSON export of the owner's full data set and streams it as a download.
//
//	@Summary		Export owner data
//	@Description	Generates a downloadable CSV or JSON file containing all of the owner's properties, rooms, tenants, contracts, bills, and payments.
//	@Tags			export
//	@Produce		text/csv
//	@Param			format	query	string	false	"Export format: csv (default) or json"
//	@Success		200		{file}	binary
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/export [get]
//	@Security		BearerAuth
func (h *SettingsHandler) ExportData(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "json" {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_FORMAT", "format must be 'csv' or 'json'"))
		return
	}

	data, contentType, err := h.svc.ExportData(c.Request.Context(), ownerID, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("EXPORT_ERROR", err.Error()))
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	ext := "csv"
	if format == "json" {
		ext = "json"
	}
	filename := fmt.Sprintf("kosku_export_%s.%s", timestamp, ext)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(data)
}
