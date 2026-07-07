package handler

import (
	"fmt"
	"io"

	"github.com/gin-gonic/gin"
)

const (
	maxUploadMemory = 10 << 20 // 10 MB
	maxFileSize     = 5 << 20  // 5 MB per file
	maxFiles        = 3
)

// Attachment represents a file attachment parsed from a multipart form.
type Attachment struct {
	Filename    string
	Data        []byte
	ContentType string
}

// parseAttachments reads up to maxFiles photo attachments from the multipart form.
// Each file must be under maxFileSize. Returns the file data and content types.
func parseAttachments(c *gin.Context, maxFiles int) ([]Attachment, error) {
	if maxFiles <= 0 {
		maxFiles = 3
	}

	var attachments []Attachment

	for i := 0; i < maxFiles; i++ {
		key := fmt.Sprintf("photos[%d]", i)
		file, header, err := c.Request.FormFile(key)
		if err != nil {
			break // No more files
		}
		defer file.Close()

		// Check file size
		if header.Size > maxFileSize {
			return nil, fmt.Errorf("file %s exceeds maximum size of %d bytes", header.Filename, maxFileSize)
		}

		// Read file data
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", header.Filename, err)
		}

		// Get content type
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		attachments = append(attachments, Attachment{
			Filename:    header.Filename,
			Data:        data,
			ContentType: contentType,
		})
	}

	return attachments, nil
}

// parseTicketAttachments is a convenience wrapper for ticket photo uploads.
func parseTicketAttachments(c *gin.Context) ([]Attachment, error) {
	return parseAttachments(c, maxFiles)
}
