package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/storage"
)

const (
	// roomPhotosBucket is the Supabase Storage bucket for room photos.
	roomPhotosBucket = "room-photos"

	// maxPhotoSize is the maximum allowed file size for room photos (5 MB).
	maxPhotoSize = 5 * 1024 * 1024
)

// allowedMIMETypes is the set of accepted MIME types for room photos.
var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// RoomPhotoService handles business logic for room photo management.
type RoomPhotoService struct {
	queries       *repository.Queries
	storageClient *storage.Client
}

// NewRoomPhotoService creates a new RoomPhotoService.
func NewRoomPhotoService(queries *repository.Queries, storageClient *storage.Client) *RoomPhotoService {
	return &RoomPhotoService{
		queries:       queries,
		storageClient: storageClient,
	}
}

// UploadPhoto validates and uploads a room photo, then inserts a room_photos row.
// It enforces that the authenticated owner owns the property that contains the room.
// fileData is the raw file bytes; declaredContentType is the Content-Type from the
// multipart header. The actual MIME type is sniffed from the first 512 bytes.
func (s *RoomPhotoService) UploadPhoto(
	ctx context.Context,
	ownerID, roomID uuid.UUID,
	fileData []byte,
	declaredContentType string,
) (dto.RoomPhotoResponse, error) {
	// 1. Validate file size.
	if len(fileData) > maxPhotoSize {
		return dto.RoomPhotoResponse{}, ErrFileTooLarge
	}

	// 2. Detect actual MIME type by sniffing the first 512 bytes.
	sniffBuf := fileData
	if len(sniffBuf) > 512 {
		sniffBuf = sniffBuf[:512]
	}
	detectedMIME := http.DetectContentType(sniffBuf)

	// Normalise: http.DetectContentType may return "image/jpeg" or "image/png" etc.
	// Accept if either the declared or detected type is in the allowlist.
	if !allowedMIMETypes[detectedMIME] && !allowedMIMETypes[declaredContentType] {
		return dto.RoomPhotoResponse{}, ErrInvalidFileType
	}

	// Use the detected MIME type for the upload; fall back to declared if detection
	// returns a generic type.
	mimeType := detectedMIME
	if mimeType == "application/octet-stream" && allowedMIMETypes[declaredContentType] {
		mimeType = declaredContentType
	}

	// 3. Verify the room exists and the owner owns the property.
	room, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomPhotoResponse{}, ErrNotFound
		}
		return dto.RoomPhotoResponse{}, fmt.Errorf("upload photo: get room: %w", err)
	}

	prop, err := s.queries.GetProperty(ctx, room.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RoomPhotoResponse{}, ErrNotFound
		}
		return dto.RoomPhotoResponse{}, fmt.Errorf("upload photo: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.RoomPhotoResponse{}, ErrForbidden
	}

	// 4. Generate a UUID-based filename with an appropriate extension.
	ext := mimeExtension(mimeType)
	filename := uuid.New().String() + ext

	// 5. Upload to Supabase Storage.
	publicURL, err := s.storageClient.UploadFile(ctx, roomPhotosBucket, filename, fileData, mimeType)
	if err != nil {
		return dto.RoomPhotoResponse{}, fmt.Errorf("upload photo: storage upload: %w", err)
	}

	// 6. Insert room_photos row.
	photo, err := s.queries.CreateRoomPhoto(ctx, repository.CreateRoomPhotoParams{
		RoomID:   roomID,
		Url:      publicURL,
		OrderIdx: sql.NullInt32{}, // default order
	})
	if err != nil {
		// Best-effort cleanup: delete the uploaded file if DB insert fails.
		_ = s.storageClient.DeleteFile(ctx, roomPhotosBucket, filename)
		return dto.RoomPhotoResponse{}, fmt.Errorf("upload photo: insert row: %w", err)
	}

	return roomPhotoToDTO(photo), nil
}

// DeletePhoto deletes a room photo from storage and the database.
// It enforces that the photo belongs to the given room and that the owner
// owns the property containing the room.
func (s *RoomPhotoService) DeletePhoto(
	ctx context.Context,
	ownerID, roomID, photoID uuid.UUID,
) error {
	// 1. Fetch the photo record.
	photo, err := s.queries.GetRoomPhoto(ctx, photoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("delete photo: get photo: %w", err)
	}

	// 2. Verify the photo belongs to the specified room.
	if photo.RoomID != roomID {
		return ErrNotFound
	}

	// 3. Verify the owner owns the property containing the room.
	room, err := s.queries.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("delete photo: get room: %w", err)
	}

	prop, err := s.queries.GetProperty(ctx, room.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("delete photo: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return ErrForbidden
	}

	// 4. Extract the filename from the public URL and delete from storage.
	filename := storage.ExtractFilenameFromURL(photo.Url, roomPhotosBucket)
	if filename != "" {
		if err := s.storageClient.DeleteFile(ctx, roomPhotosBucket, filename); err != nil {
			// Log but don't fail — the DB row should still be removed.
			// In production this would be logged via the logger package.
			_ = err
		}
	}

	// 5. Delete the DB row.
	if err := s.queries.DeleteRoomPhoto(ctx, photoID); err != nil {
		return fmt.Errorf("delete photo: delete row: %w", err)
	}

	return nil
}

// ErrFileTooLarge is returned when an uploaded file exceeds the size limit.
var ErrFileTooLarge = errors.New("file too large: maximum size is 5MB")

// ErrInvalidFileType is returned when an uploaded file has a disallowed MIME type.
var ErrInvalidFileType = errors.New("invalid file type: only JPEG, PNG, and WebP images are allowed")

// roomPhotoToDTO converts a repository.RoomPhoto to a dto.RoomPhotoResponse.
func roomPhotoToDTO(p repository.RoomPhoto) dto.RoomPhotoResponse {
	resp := dto.RoomPhotoResponse{
		ID:     p.ID.String(),
		RoomID: p.RoomID.String(),
		URL:    p.Url,
	}
	if p.OrderIdx.Valid {
		resp.OrderIdx = int(p.OrderIdx.Int32)
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// mimeExtension returns a file extension for the given MIME type.
func mimeExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
