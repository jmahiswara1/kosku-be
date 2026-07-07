package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/service"
)

// TicketServicer is the interface that TicketHandler depends on.
type TicketServicer interface {
	CreateTicket(ctx context.Context, tenantID uuid.UUID, req dto.CreateTicketRequest, photos [][]byte, photoContentTypes []string) (dto.TicketResponse, error)
	ListTickets(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, priority string, page, perPage int) ([]dto.TicketResponse, int64, error)
	GetTicket(ctx context.Context, callerID uuid.UUID, callerRole string, ticketID uuid.UUID) (dto.TicketResponse, error)
	UpdateTicket(ctx context.Context, ownerID uuid.UUID, ticketID uuid.UUID, req dto.UpdateTicketRequest) (dto.TicketResponse, error)
}

// Ensure *service.TicketService satisfies TicketServicer at compile time.
var _ TicketServicer = (*service.TicketService)(nil)

// TicketHandler holds the dependencies for ticket-related HTTP handlers.
type TicketHandler struct {
	svc TicketServicer
}

// NewTicketHandler creates a new TicketHandler.
func NewTicketHandler(svc *service.TicketService) *TicketHandler {
	return &TicketHandler{svc: svc}
}

// NewTicketHandlerWithService creates a new TicketHandler with any TicketServicer.
// Intended for use in tests.
func NewTicketHandlerWithService(svc TicketServicer) *TicketHandler {
	return &TicketHandler{svc: svc}
}

// CreateTicket handles POST /v1/tickets (tenant).
// Accepts a multipart form with title, description, optional priority, and up to
// 3 photo attachments (5MB each). Uploads photos to Supabase Storage, inserts
// ticket + attachment rows, notifies the owner, and sends an email.
//
//	@Summary		Submit a complaint ticket
//	@Description	Creates a new complaint ticket for the authenticated tenant with optional photo attachments.
//	@Tags			tickets
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			title		formData	string	true	"Ticket title"
//	@Param			description	formData	string	true	"Ticket description"
//	@Param			priority	formData	string	false	"Priority (low, medium, high, urgent)"
//	@Param			photos		formData	file	false	"Photo attachments (max 3, 5MB each)"
//	@Success		201			{object}	map[string]interface{}
//	@Failure		400			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/tickets [post]
//	@Security		BearerAuth
func (h *TicketHandler) CreateTicket(c *gin.Context) {
	tenantIDStr := c.GetString(middleware.ContextKeyUserID)
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
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

	ticket, err := h.svc.CreateTicket(c.Request.Context(), tenantID, req, photos, photoContentTypes)
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

// ListTickets handles GET /v1/tickets (owner).
// Returns tickets for a property with optional status/priority filters and pagination.
//
//	@Summary		List complaint tickets
//	@Description	Returns all complaint tickets for a property with optional filters and pagination.
//	@Tags			tickets
//	@Produce		json
//	@Param			property_id	query	string	true	"Property UUID"
//	@Param			status		query	string	false	"Filter by status (open, in_progress, resolved)"
//	@Param			priority	query	string	false	"Filter by priority (low, medium, high, urgent)"
//	@Param			page		query	int		false	"Page number (default 1)"
//	@Param			per_page	query	int		false	"Items per page (default 20)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	map[string]interface{}
//	@Failure		403			{object}	map[string]interface{}
//	@Router			/tickets [get]
//	@Security		BearerAuth
func (h *TicketHandler) ListTickets(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyIDStr := c.Query("property_id")
	if propertyIDStr == "" {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "property_id is required"))
		return
	}
	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property_id"))
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

	tickets, total, err := h.svc.ListTickets(
		c.Request.Context(),
		ownerID,
		propertyID,
		c.Query("status"),
		c.Query("priority"),
		page,
		perPage,
	)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
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

// GetTicket handles GET /v1/tickets/:id.
// Returns ticket detail with attachments. Accessible by both owners and tenants.
//
//	@Summary		Get ticket detail
//	@Description	Returns a single ticket with all attachments. Accessible by the owning property's owner or the submitting tenant.
//	@Tags			tickets
//	@Produce		json
//	@Param			id	path		string	true	"Ticket UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/tickets/{id} [get]
//	@Security		BearerAuth
func (h *TicketHandler) GetTicket(c *gin.Context) {
	callerIDStr := c.GetString(middleware.ContextKeyUserID)
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}
	callerRole := c.GetString(middleware.ContextKeyRole)

	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid ticket ID"))
		return
	}

	ticket, err := h.svc.GetTicket(c.Request.Context(), callerID, callerRole, ticketID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Ticket not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not have access to this ticket"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_TICKET_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(ticket))
}

// UpdateTicket handles PUT /v1/tickets/:id (owner).
// Updates ticket status, priority, and resolution. Notifies the tenant and writes an audit log.
//
//	@Summary		Update a complaint ticket
//	@Description	Updates a ticket's status, priority, and resolution note. Notifies the tenant via in-app notification and email.
//	@Tags			tickets
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Ticket UUID"
//	@Param			body	body		dto.UpdateTicketRequest		true	"Update data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/tickets/{id} [put]
//	@Security		BearerAuth
func (h *TicketHandler) UpdateTicket(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid ticket ID"))
		return
	}

	var req dto.UpdateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	ticket, err := h.svc.UpdateTicket(c.Request.Context(), ownerID, ticketID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Ticket not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this ticket's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_TICKET_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(ticket))
}
