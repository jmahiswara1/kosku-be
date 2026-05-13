package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// RoomServicer is the interface that RoomHandler depends on.
// It is satisfied by *service.RoomService and can be implemented by test mocks.
type RoomServicer interface {
	ListRooms(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error)
	CreateRoom(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error)
	GetRoom(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error)
	UpdateRoom(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error)
	ArchiveRoom(ctx context.Context, ownerID, roomID uuid.UUID) error
	UpdateLayout(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error
	GetRoomHistory(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error)
}

// Ensure *service.RoomService satisfies RoomServicer at compile time.
var _ RoomServicer = (*service.RoomService)(nil)

// RoomHandler holds the dependencies for room-related HTTP handlers.
type RoomHandler struct {
	svc RoomServicer
}

// NewRoomHandler creates a new RoomHandler backed by a *service.RoomService.
func NewRoomHandler(svc *service.RoomService) *RoomHandler {
	return &RoomHandler{svc: svc}
}

// NewRoomHandlerWithService creates a new RoomHandler with any RoomServicer.
// Intended for use in tests.
func NewRoomHandlerWithService(svc RoomServicer) *RoomHandler {
	return &RoomHandler{svc: svc}
}

// ListRooms handles GET /v1/properties/:id/rooms.
// Returns all non-archived rooms for the property with status and type.
//
//	@Summary		List rooms in a property
//	@Description	Returns all rooms for the given property, including room type and status.
//	@Tags			rooms
//	@Produce		json
//	@Param			id	path		string	true	"Property UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/properties/{id}/rooms [get]
//	@Security		BearerAuth
func (h *RoomHandler) ListRooms(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	rooms, err := h.svc.ListRooms(c.Request.Context(), ownerID, propertyID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_ROOMS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rooms,
		"meta":    gin.H{"total": len(rooms)},
	})
}

// CreateRoom handles POST /v1/properties/:id/rooms.
// Validates room number uniqueness within the property and inserts the room.
//
//	@Summary		Create a room
//	@Description	Creates a new room in the given property. Creates a new room type if the name doesn't exist.
//	@Tags			rooms
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Property UUID"
//	@Param			body	body		dto.CreateRoomRequest	true	"Room data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/properties/{id}/rooms [post]
//	@Security		BearerAuth
func (h *RoomHandler) CreateRoom(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	var req dto.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	room, err := h.svc.CreateRoom(c.Request.Context(), ownerID, propertyID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		if errors.Is(err, service.ErrDuplicateRoomNumber) {
			c.JSON(http.StatusConflict, errorResponse("DUPLICATE_ROOM_NUMBER", "Room number already exists in this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("CREATE_ROOM_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(room))
}

// GetRoom handles GET /v1/rooms/:id.
// Returns room detail; enforces owner ownership.
//
//	@Summary		Get room detail
//	@Description	Returns a single room with type and facilities. Only accessible by the owning user.
//	@Tags			rooms
//	@Produce		json
//	@Param			id	path		string	true	"Room UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/rooms/{id} [get]
//	@Security		BearerAuth
func (h *RoomHandler) GetRoom(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	room, err := h.svc.GetRoom(c.Request.Context(), ownerID, roomID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Room not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_ROOM_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(room))
}

// UpdateRoom handles PUT /v1/rooms/:id.
// Updates room fields; enforces owner ownership.
//
//	@Summary		Update a room
//	@Description	Updates a room's details. Only accessible by the owning user.
//	@Tags			rooms
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Room UUID"
//	@Param			body	body		dto.UpdateRoomRequest	true	"Updated room data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/rooms/{id} [put]
//	@Security		BearerAuth
func (h *RoomHandler) UpdateRoom(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	var req dto.UpdateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	room, err := h.svc.UpdateRoom(c.Request.Context(), ownerID, roomID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Room not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room"))
			return
		}
		if errors.Is(err, service.ErrDuplicateRoomNumber) {
			c.JSON(http.StatusConflict, errorResponse("DUPLICATE_ROOM_NUMBER", "Room number already exists in this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_ROOM_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(room))
}

// DeleteRoom handles DELETE /v1/rooms/:id.
// Soft-archives the room (sets archived_at); enforces owner ownership.
//
//	@Summary		Archive a room
//	@Description	Soft-archives a room by setting archived_at. Only accessible by the owning user.
//	@Tags			rooms
//	@Produce		json
//	@Param			id	path		string	true	"Room UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/rooms/{id} [delete]
//	@Security		BearerAuth
func (h *RoomHandler) DeleteRoom(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	if err := h.svc.ArchiveRoom(c.Request.Context(), ownerID, roomID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Room not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("ARCHIVE_ROOM_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Room archived successfully"},
	})
}

// UpdateLayout handles PUT /v1/properties/:id/layout.
// Batch-updates grid_x/grid_y for all rooms in the property in a single transaction.
//
//	@Summary		Save room grid layout
//	@Description	Batch-updates grid positions for rooms in a property. All updates run in a single transaction.
//	@Tags			rooms
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Property UUID"
//	@Param			body	body		dto.UpdateLayoutRequest	true	"Layout data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/properties/{id}/layout [put]
//	@Security		BearerAuth
func (h *RoomHandler) UpdateLayout(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	var req dto.UpdateLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	if err := h.svc.UpdateLayout(c.Request.Context(), ownerID, propertyID, req); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_LAYOUT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Layout updated successfully"},
	})
}

// GetRoomHistory handles GET /v1/rooms/:id/history.
// Returns past contracts for the room ordered by start_date DESC.
//
//	@Summary		Get room tenancy history
//	@Description	Returns all past contracts for a room, ordered by start_date DESC.
//	@Tags			rooms
//	@Produce		json
//	@Param			id	path		string	true	"Room UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/rooms/{id}/history [get]
//	@Security		BearerAuth
func (h *RoomHandler) GetRoomHistory(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	history, err := h.svc.GetRoomHistory(c.Request.Context(), ownerID, roomID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Room not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_ROOM_HISTORY_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    history,
		"meta":    gin.H{"total": len(history)},
	})
}
