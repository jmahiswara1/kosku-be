package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/service"
)

// TenantPortalServicer is the interface that TenantPortalHandler depends on.
type TenantPortalServicer interface {
	GetMyRoom(ctx context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error)
	ListMyBills(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.BillResponse, int64, error)
	GetBillReceipt(ctx context.Context, tenantID, billID uuid.UUID) ([]byte, string, error)
	ListMyTickets(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.TicketResponse, int64, error)
	CreateMyTicket(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error)
	ListMyContracts(ctx context.Context, tenantID uuid.UUID) ([]dto.ContractResponse, error)
	RequestContractRenewal(ctx context.Context, tenantID uuid.UUID, req dto.ContractRenewalRequest) error
	ListExpiringContracts(ctx context.Context, days int) ([]dto.ContractResponse, error)
}

// Ensure *service.TenantPortalService satisfies TenantPortalServicer at compile time.
var _ TenantPortalServicer = (*service.TenantPortalService)(nil)

// TenantPortalHandler holds the dependencies for tenant portal HTTP handlers.
type TenantPortalHandler struct {
	svc TenantPortalServicer
}

// NewTenantPortalHandler creates a new TenantPortalHandler.
func NewTenantPortalHandler(svc *service.TenantPortalService) *TenantPortalHandler {
	return &TenantPortalHandler{svc: svc}
}

// NewTenantPortalHandlerWithService creates a new TenantPortalHandler with any TenantPortalServicer.
// Intended for use in tests.
func NewTenantPortalHandlerWithService(svc TenantPortalServicer) *TenantPortalHandler {
	return &TenantPortalHandler{svc: svc}
}

// tenantIDFromContext extracts and parses the tenant UUID from the Gin context.
func tenantIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	idStr := c.GetString(middleware.ContextKeyUserID)
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return uuid.UUID{}, false
	}
	return id, true
}

// GetMyRoom handles GET /v1/me/room.
// Returns the tenant's current room details and active contract.
//
//	@Summary		Get my room
//	@Description	Returns the authenticated tenant's current room details and active contract.
//	@Tags			tenant-portal
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/me/room [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) GetMyRoom(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		return
	}

	room, err := h.svc.GetMyRoom(c.Request.Context(), tenantID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "No room assigned to this tenant"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_ROOM_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(room))
}

// ListMyBills handles GET /v1/me/bills.
// Returns the tenant's own bills with status and due dates.
//
//	@Summary		List my bills
//	@Description	Returns all bills for the authenticated tenant with optional status filter and pagination.
//	@Tags			tenant-portal
//	@Produce		json
//	@Param			status		query	string	false	"Filter by status (unpaid, pending, paid, overdue)"
//	@Param			page		query	int		false	"Page number (default 1)"
//	@Param			per_page	query	int		false	"Items per page (default 20)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/me/bills [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) ListMyBills(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
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

	bills, total, err := h.svc.ListMyBills(c.Request.Context(), tenantID, c.Query("status"), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_BILLS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    bills,
		"meta": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
		},
	})
}

// GetBillReceipt handles GET /v1/me/bills/:id/receipt.
// Generates and streams a PDF receipt for a paid bill.
//
//	@Summary		Download bill receipt
//	@Description	Generates and streams a PDF receipt for a paid bill. Only available for bills with status 'paid'.
//	@Tags			tenant-portal
//	@Produce		application/pdf
//	@Param			id	path		string	true	"Bill UUID"
//	@Success		200	{file}		binary
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/me/bills/{id}/receipt [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) GetBillReceipt(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		return
	}

	billID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid bill ID"))
		return
	}

	pdfBytes, filename, err := h.svc.GetBillReceipt(c.Request.Context(), tenantID, billID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Bill not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "This bill does not belong to you"))
			return
		}
		if errors.Is(err, service.ErrBillNotPaid) {
			c.JSON(http.StatusBadRequest, errorResponse("BILL_NOT_PAID", "Receipt is only available for paid bills"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("RECEIPT_ERROR", err.Error()))
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Length", strconv.Itoa(len(pdfBytes)))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, bytes.NewReader(pdfBytes))
}

// ListMyTickets handles GET /v1/me/tickets.
// Returns the tenant's own complaint tickets.
//
//	@Summary		List my tickets
//	@Description	Returns all complaint tickets submitted by the authenticated tenant.
//	@Tags			tenant-portal
//	@Produce		json
//	@Param			status		query	string	false	"Filter by status (open, in_progress, resolved)"
//	@Param			page		query	int		false	"Page number (default 1)"
//	@Param			per_page	query	int		false	"Items per page (default 20)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/me/tickets [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) ListMyTickets(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
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

	tickets, total, err := h.svc.ListMyTickets(c.Request.Context(), tenantID, c.Query("status"), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_TICKETS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tickets,
		"meta": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
		},
	})
}

// CreateMyTicket handles POST /v1/me/tickets.
// Accepts a multipart form with title, description, optional priority, and up to
// 3 photo attachments (5MB each).
//
//	@Summary		Submit a complaint ticket
//	@Description	Creates a new complaint ticket for the authenticated tenant with optional photo attachments.
//	@Tags			tenant-portal
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			title		formData	string	true	"Ticket title"
//	@Param			description	formData	string	true	"Ticket description"
//	@Param			priority	formData	string	false	"Priority (low, medium, high, urgent)"
//	@Param			photos		formData	file	false	"Photo attachments (max 3, 5MB each)"
//	@Success		201			{object}	map[string]interface{}
//	@Failure		400			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/me/tickets [post]
//	@Security		BearerAuth
func (h *TenantPortalHandler) CreateMyTicket(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		return
	}

	// Parse multipart form.
	if err := c.Request.ParseMultipartForm(maxUploadMemory); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_FORM", "Failed to parse multipart form"))
		return
	}

	var req dto.CreateTicketRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	attachments, err := parseTicketAttachments(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_FORM", err.Error()))
		return
	}

	// Extract data from attachments.
	photos := make([][]byte, len(attachments))
	photoContentTypes := make([]string, len(attachments))
	for i, att := range attachments {
		photos[i] = att.Data
		photoContentTypes[i] = att.ContentType
	}

	ticket, err := h.svc.CreateMyTicket(c.Request.Context(), tenantID, req, photos, photoContentTypes)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant or property not found"))
			return
		}
		if errors.Is(err, service.ErrTooManyAttachments) {
			c.JSON(http.StatusBadRequest, errorResponse("TOO_MANY_ATTACHMENTS", err.Error()))
			return
		}
		if errors.Is(err, service.ErrFileTooLarge) {
			c.JSON(http.StatusBadRequest, errorResponse("FILE_TOO_LARGE", err.Error()))
			return
		}
		if errors.Is(err, service.ErrInvalidFileType) {
			c.JSON(http.StatusBadRequest, errorResponse("INVALID_FILE_TYPE", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("CREATE_TICKET_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(ticket))
}

// ListMyContracts handles GET /v1/me/contracts.
// Returns all contracts for the authenticated tenant.
//
//	@Summary		List my contracts
//	@Description	Returns all contracts (active and historical) for the authenticated tenant.
//	@Tags			tenant-portal
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/me/contracts [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) ListMyContracts(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		return
	}

	contracts, err := h.svc.ListMyContracts(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_CONTRACTS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    contracts,
		"meta":    gin.H{"total": len(contracts)},
	})
}

// RequestContractRenewal handles POST /v1/me/contracts/renew.
// Submits a contract renewal request and creates a notification for the owner.
//
//	@Summary		Request contract renewal
//	@Description	Submits a contract renewal request. Creates an in-app notification for the property owner.
//	@Tags			tenant-portal
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.ContractRenewalRequest	false	"Renewal request details"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/me/contracts/renew [post]
//	@Security		BearerAuth
func (h *TenantPortalHandler) RequestContractRenewal(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		return
	}

	var req dto.ContractRenewalRequest
	// Body is optional — ignore bind errors.
	_ = c.ShouldBindJSON(&req)

	if err := h.svc.RequestContractRenewal(c.Request.Context(), tenantID, req); err != nil {
		if errors.Is(err, service.ErrNoActiveContract) {
			c.JSON(http.StatusNotFound, errorResponse("NO_ACTIVE_CONTRACT", "No active contract found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("RENEWAL_REQUEST_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Renewal request submitted successfully. The owner has been notified."},
	})
}

// ListExpiringContracts handles GET /v1/contracts?expiring=true&days=30.
// Returns contracts expiring within the specified number of days (default 30).
//
//	@Summary		List expiring contracts
//	@Description	Returns active contracts expiring within the given number of days. Owner only.
//	@Tags			contracts
//	@Produce		json
//	@Param			days	query	int	false	"Number of days to look ahead (default 30)"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/contracts [get]
//	@Security		BearerAuth
func (h *TenantPortalHandler) ListExpiringContracts(c *gin.Context) {
	days := 30 // default
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	contracts, err := h.svc.ListExpiringContracts(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_EXPIRING_ERROR", "Failed to list expiring contracts"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": contracts})
}
