package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// maxUploadMemory is the maximum amount of memory used when parsing multipart
// form data. Files larger than this are stored in temporary files.
const maxUploadMemory = 10 * 1024 * 1024 // 10 MB

// RoomPhotoServicer is the interface that RoomPhotoHandler depends on.
type RoomPhotoServicer interface {
	UploadPhoto(ctx context.Context, ownerID, roomID uuid.UUID, fileData []byte, declaredContentType string) (dto.RoomPhotoResponse, error)
	DeletePhoto(ctx context.Context, ownerID, roomID, photoID uuid.UUID) error
}

// Ensure *service.RoomPhotoService satisfies RoomPhotoServicer at compile time.
var _ RoomPhotoServicer = (*service.RoomPhotoService)(nil)

// RoomPhotoHandler holds the dependencies for room photo HTTP handlers.
type RoomPhotoHandler struct {
	svc RoomPhotoServicer
}

// NewRoomPhotoHandler creates a new RoomPhotoHandler.
func NewRoomPhotoHandler(svc *service.RoomPhotoService) *RoomPhotoHandler {
	return &RoomPhotoHandler{svc: svc}
}

// NewRoomPhotoHandlerWithService creates a new RoomPhotoHandler with any RoomPhotoServicer.
// Intended for use in tests.
func NewRoomPhotoHandlerWithService(svc RoomPhotoServicer) *RoomPhotoHandler {
	return &RoomPhotoHandler{svc: svc}
}

// UploadPhoto handles POST /v1/rooms/:id/photos.
// Accepts a multipart form with a "photo" file field.
// Validates MIME type (jpeg/png/webp) and size (≤5MB), uploads to Supabase
// Storage bucket "room-photos" with a UUID filename, and inserts a room_photos row.
//
//	@Summary		Upload a room photo
//	@Description	Accepts a multipart form upload, validates the file, stores it in Supabase Storage, and records the URL in the database.
//	@Tags			rooms
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			id		path		string	true	"Room UUID"
//	@Param			photo	formData	file	true	"Photo file (JPEG, PNG, or WebP; max 5MB)"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/rooms/{id}/photos [post]
//	@Security		BearerAuth
func (h *RoomPhotoHandler) UploadPhoto(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	// Parse the multipart form.
	if err := c.Request.ParseMultipartForm(maxUploadMemory); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_FORM", "Failed to parse multipart form"))
		return
	}

	file, header, err := c.Request.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("MISSING_FILE", "A 'photo' file field is required"))
		return
	}
	defer file.Close()

	// Read the file bytes.
	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("READ_FILE_ERROR", "Failed to read uploaded file"))
		return
	}

	// Get the declared Content-Type from the multipart header.
	declaredContentType := header.Header.Get("Content-Type")

	photo, err := h.svc.UploadPhoto(c.Request.Context(), ownerID, roomID, fileData, declaredContentType)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Room not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room's property"))
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
		c.JSON(http.StatusInternalServerError, errorResponse("UPLOAD_PHOTO_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(photo))
}

// DeletePhoto handles DELETE /v1/rooms/:id/photos/:photoId.
// Deletes the photo from Supabase Storage and removes the room_photos row.
//
//	@Summary		Delete a room photo
//	@Description	Deletes a room photo from storage and the database. Enforces ownership.
//	@Tags			rooms
//	@Produce		json
//	@Param			id		path		string	true	"Room UUID"
//	@Param			photoId	path		string	true	"Photo UUID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/rooms/{id}/photos/{photoId} [delete]
//	@Security		BearerAuth
func (h *RoomPhotoHandler) DeletePhoto(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid room ID"))
		return
	}

	photoID, err := uuid.Parse(c.Param("photoId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid photo ID"))
		return
	}

	if err := h.svc.DeletePhoto(c.Request.Context(), ownerID, roomID, photoID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Photo not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this room's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("DELETE_PHOTO_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Photo deleted successfully"},
	})
}
