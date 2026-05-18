package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// AnnouncementServicer is the interface that AnnouncementHandler depends on.
type AnnouncementServicer interface {
	CreateAnnouncement(ctx context.Context, ownerID uuid.UUID, req dto.CreateAnnouncementRequest) (dto.AnnouncementResponse, error)
}

// Ensure *service.AnnouncementService satisfies AnnouncementServicer at compile time.
var _ AnnouncementServicer = (*service.AnnouncementService)(nil)

// AnnouncementHandler holds the dependencies for announcement-related HTTP handlers.
type AnnouncementHandler struct {
	svc AnnouncementServicer
}

// NewAnnouncementHandler creates a new AnnouncementHandler.
func NewAnnouncementHandler(svc *service.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{svc: svc}
}

// NewAnnouncementHandlerWithService creates a new AnnouncementHandler with any
// AnnouncementServicer. Intended for use in tests.
func NewAnnouncementHandlerWithService(svc AnnouncementServicer) *AnnouncementHandler {
	return &AnnouncementHandler{svc: svc}
}

// CreateAnnouncement handles POST /v1/announcements.
// Inserts an announcement row, fans out notification rows to targeted tenants,
// and optionally sends email via Resend.
//
//	@Summary		Create announcement
//	@Description	Creates an announcement and fans out in-app notifications (and optionally emails) to targeted tenants.
//	@Tags			announcements
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CreateAnnouncementRequest	true	"Announcement data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/announcements [post]
//	@Security		BearerAuth
func (h *AnnouncementHandler) CreateAnnouncement(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	ann, err := h.svc.CreateAnnouncement(c.Request.Context(), ownerID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("CREATE_ANNOUNCEMENT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    ann,
		"meta":    gin.H{"notified_count": len(ann.NotifiedTenants)},
	})
}
