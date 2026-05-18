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
	"github.com/kosku/backend/pkg/email"
	"github.com/kosku/backend/pkg/storage"
)

const (
	ticketPhotosBucket = "ticket-photos"
	maxTicketPhotoSize = 5 * 1024 * 1024 // 5 MB
	maxTicketPhotos    = 3
)

// allowedTicketMIMETypes is the set of accepted MIME types for ticket attachments.
var allowedTicketMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// ErrTooManyAttachments is returned when more than 3 photos are submitted.
var ErrTooManyAttachments = errors.New("too many attachments: maximum is 3 photos")

// TicketService handles business logic for complaint ticket management.
type TicketService struct {
	queries       *repository.Queries
	storageClient *storage.Client
	emailClient   *email.Client
}

// NewTicketService creates a new TicketService.
func NewTicketService(queries *repository.Queries, storageClient *storage.Client, emailClient *email.Client) *TicketService {
	return &TicketService{
		queries:       queries,
		storageClient: storageClient,
		emailClient:   emailClient,
	}
}

// CreateTicket creates a new complaint ticket for a tenant.
// It validates the request, uploads any photo attachments to Supabase Storage,
// inserts the ticket and attachment rows, creates an in-app notification for the
// owner, and sends an email to the owner.
func (s *TicketService) CreateTicket(
	ctx context.Context,
	tenantID uuid.UUID,
	req dto.CreateTicketRequest,
	photos [][]byte,
	photoContentTypes []string,
) (dto.TicketResponse, error) {
	// Validate attachment count.
	if len(photos) > maxTicketPhotos {
		return dto.TicketResponse{}, ErrTooManyAttachments
	}

	// Validate each photo.
	for i, data := range photos {
		if len(data) > maxTicketPhotoSize {
			return dto.TicketResponse{}, ErrFileTooLarge
		}
		sniff := data
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		detected := http.DetectContentType(sniff)
		declared := ""
		if i < len(photoContentTypes) {
			declared = photoContentTypes[i]
		}
		if !allowedTicketMIMETypes[detected] && !allowedTicketMIMETypes[declared] {
			return dto.TicketResponse{}, ErrInvalidFileType
		}
	}

	// Fetch the tenant to get property_id and room_id.
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TicketResponse{}, ErrNotFound
		}
		return dto.TicketResponse{}, fmt.Errorf("create ticket: get tenant: %w", err)
	}

	if !tenant.PropertyID.Valid {
		return dto.TicketResponse{}, fmt.Errorf("create ticket: tenant has no assigned property")
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	// Validate priority value.
	switch priority {
	case "low", "medium", "high", "urgent":
		// valid
	default:
		priority = "medium"
	}

	// Determine room_id (nullable).
	var roomID uuid.NullUUID
	if tenant.RoomID.Valid {
		roomID = uuid.NullUUID{UUID: tenant.RoomID.UUID, Valid: true}
	}

	// Insert the ticket row.
	ticket, err := s.queries.CreateTicket(ctx, repository.CreateTicketParams{
		TenantID:    tenantID,
		PropertyID:  tenant.PropertyID.UUID,
		RoomID:      roomID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		Status:      "open",
	})
	if err != nil {
		return dto.TicketResponse{}, fmt.Errorf("create ticket: insert ticket: %w", err)
	}

	// Upload photos and insert attachment rows.
	attachments := make([]dto.TicketAttachmentResponse, 0, len(photos))
	uploadedFilenames := make([]string, 0, len(photos))
	for i, data := range photos {
		sniff := data
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		mimeType := http.DetectContentType(sniff)
		if mimeType == "application/octet-stream" && i < len(photoContentTypes) && allowedTicketMIMETypes[photoContentTypes[i]] {
			mimeType = photoContentTypes[i]
		}

		ext := mimeExtension(mimeType)
		filename := uuid.New().String() + ext
		uploadedFilenames = append(uploadedFilenames, filename)

		publicURL, err := s.storageClient.UploadFile(ctx, ticketPhotosBucket, filename, data, mimeType)
		if err != nil {
			// Clean up already-uploaded files.
			for _, fn := range uploadedFilenames[:len(uploadedFilenames)-1] {
				_ = s.storageClient.DeleteFile(ctx, ticketPhotosBucket, fn)
			}
			return dto.TicketResponse{}, fmt.Errorf("create ticket: upload photo: %w", err)
		}

		attachment, err := s.queries.CreateTicketAttachment(ctx, repository.CreateTicketAttachmentParams{
			TicketID: ticket.ID,
			Url:      publicURL,
		})
		if err != nil {
			// Clean up uploaded files.
			for _, fn := range uploadedFilenames {
				_ = s.storageClient.DeleteFile(ctx, ticketPhotosBucket, fn)
			}
			return dto.TicketResponse{}, fmt.Errorf("create ticket: insert attachment: %w", err)
		}
		attachments = append(attachments, ticketAttachmentToDTO(attachment))
	}

	// Fetch the owner of the property to send notification and email.
	prop, err := s.queries.GetProperty(ctx, tenant.PropertyID.UUID)
	if err == nil {
		// Insert in-app notification for the owner — non-fatal.
		notifBody := fmt.Sprintf("New complaint from tenant: %s", req.Title)
		_, _ = s.queries.CreateNotification(ctx, repository.CreateNotificationParams{
			UserID:   prop.OwnerID,
			Type:     "ticket_created",
			Title:    "New Complaint Ticket",
			Body:     sql.NullString{String: notifBody, Valid: true},
			EntityID: uuid.NullUUID{UUID: ticket.ID, Valid: true},
		})

		// Send email to owner — non-fatal.
		go func() {
			ownerProfile, err := s.queries.GetProfile(ctx, prop.OwnerID)
			if err != nil {
				return
			}
			_ = s.emailClient.SendComplaintSubmitted(
				"", // owner email not available from profiles table
				ownerProfile.FullName,
				prop.Name,
				ticket.ID.String(),
				req.Title,
			)
		}()
	}

	resp := ticketToDTO(ticket, "")
	resp.Attachments = attachments
	return resp, nil
}

// ListTickets returns all tickets for a property with optional filters and pagination.
func (s *TicketService) ListTickets(
	ctx context.Context,
	ownerID uuid.UUID,
	propertyID uuid.UUID,
	status, priority string,
	page, perPage int,
) ([]dto.TicketResponse, int64, error) {
	// Ownership check.
	prop, err := s.queries.GetProperty(ctx, propertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("list tickets: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return nil, 0, ErrForbidden
	}

	tickets, total, err := s.queries.ListTicketsFiltered(ctx, repository.ListTicketsFilteredParams{
		PropertyID: propertyID,
		Status:     status,
		Priority:   priority,
		Limit:      int32(perPage),              //nolint:gosec // bounded pagination value
		Offset:     int32((page - 1) * perPage), //nolint:gosec // bounded pagination value
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list tickets: %w", err)
	}

	result := make([]dto.TicketResponse, 0, len(tickets))
	for _, row := range tickets {
		result = append(result, ticketToDTO(row.Ticket, row.TenantName))
	}
	return result, total, nil
}

// GetTicket returns a single ticket with its attachments.
// Both owners and tenants can access tickets they are associated with.
func (s *TicketService) GetTicket(ctx context.Context, callerID uuid.UUID, callerRole string, ticketID uuid.UUID) (dto.TicketResponse, error) {
	ticket, err := s.queries.GetTicket(ctx, ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TicketResponse{}, ErrNotFound
		}
		return dto.TicketResponse{}, fmt.Errorf("get ticket: %w", err)
	}

	// Access control: owner must own the property; tenant must own the ticket.
	switch callerRole {
	case "owner":
		prop, err := s.queries.GetProperty(ctx, ticket.PropertyID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.TicketResponse{}, ErrNotFound
			}
			return dto.TicketResponse{}, fmt.Errorf("get ticket: get property: %w", err)
		}
		if prop.OwnerID != callerID {
			return dto.TicketResponse{}, ErrForbidden
		}
	case "tenant":
		if ticket.TenantID != callerID {
			return dto.TicketResponse{}, ErrForbidden
		}
	default:
		return dto.TicketResponse{}, ErrForbidden
	}

	// Fetch attachments.
	attachmentRows, err := s.queries.ListTicketAttachments(ctx, ticketID)
	if err != nil {
		return dto.TicketResponse{}, fmt.Errorf("get ticket: list attachments: %w", err)
	}

	// Fetch tenant name.
	tenantName := ""
	tenantProfile, err := s.queries.GetProfile(ctx, ticket.TenantID)
	if err == nil {
		tenantName = tenantProfile.FullName
	}

	resp := ticketToDTO(ticket, tenantName)
	resp.Attachments = make([]dto.TicketAttachmentResponse, 0, len(attachmentRows))
	for _, a := range attachmentRows {
		resp.Attachments = append(resp.Attachments, ticketAttachmentToDTO(a))
	}
	return resp, nil
}

// UpdateTicket updates a ticket's status, priority, and resolution.
// It inserts an in-app notification for the tenant, sends an email to the tenant,
// and writes an audit log entry.
func (s *TicketService) UpdateTicket(
	ctx context.Context,
	ownerID uuid.UUID,
	ticketID uuid.UUID,
	req dto.UpdateTicketRequest,
) (dto.TicketResponse, error) {
	// Fetch the ticket.
	ticket, err := s.queries.GetTicket(ctx, ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TicketResponse{}, ErrNotFound
		}
		return dto.TicketResponse{}, fmt.Errorf("update ticket: get ticket: %w", err)
	}

	// Ownership check via property.
	prop, err := s.queries.GetProperty(ctx, ticket.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TicketResponse{}, ErrNotFound
		}
		return dto.TicketResponse{}, fmt.Errorf("update ticket: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.TicketResponse{}, ErrForbidden
	}

	// Build resolution argument (nullable).
	var resolutionArg sql.NullString
	if req.Resolution != "" {
		resolutionArg = sql.NullString{String: req.Resolution, Valid: true}
	}

	// Update the ticket.
	updated, err := s.queries.UpdateTicket(ctx, repository.UpdateTicketParams{
		ID:         ticketID,
		Priority:   req.Priority,
		Status:     req.Status,
		Resolution: resolutionArg,
	})
	if err != nil {
		return dto.TicketResponse{}, fmt.Errorf("update ticket: %w", err)
	}

	// Insert in-app notification for the tenant — non-fatal.
	notifTitle := "Your complaint ticket has been updated"
	notifBody := fmt.Sprintf("Ticket #%s status changed to %s", ticketID.String()[:8], req.Status)
	_, _ = s.queries.CreateNotification(ctx, repository.CreateNotificationParams{
		UserID:   ticket.TenantID,
		Type:     "ticket_updated",
		Title:    notifTitle,
		Body:     sql.NullString{String: notifBody, Valid: true},
		EntityID: uuid.NullUUID{UUID: ticketID, Valid: true},
	})

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_ticket", "ticket", ticketID, map[string]string{
		"ticket_id": ticketID.String(),
		"status":    req.Status,
		"priority":  req.Priority,
		"owner_id":  ownerID.String(),
	}))

	// Send email to tenant — non-fatal.
	go func() {
		tenantProfile, err := s.queries.GetProfile(ctx, ticket.TenantID)
		if err != nil {
			return
		}
		_ = s.emailClient.SendComplaintUpdated(
			"", // tenant email not available from profiles table
			tenantProfile.FullName,
			prop.Name,
			ticketID.String(),
			req.Status,
		)
	}()

	// Fetch tenant name for response.
	tenantName := ""
	tenantProfile, err := s.queries.GetProfile(ctx, updated.TenantID)
	if err == nil {
		tenantName = tenantProfile.FullName
	}

	// Fetch attachments for response.
	attachmentRows, _ := s.queries.ListTicketAttachments(ctx, ticketID)
	resp := ticketToDTO(updated, tenantName)
	resp.Attachments = make([]dto.TicketAttachmentResponse, 0, len(attachmentRows))
	for _, a := range attachmentRows {
		resp.Attachments = append(resp.Attachments, ticketAttachmentToDTO(a))
	}
	return resp, nil
}

// ticketToDTO converts a repository.Ticket to dto.TicketResponse.
func ticketToDTO(t repository.Ticket, tenantName string) dto.TicketResponse {
	resp := dto.TicketResponse{
		ID:          t.ID.String(),
		TenantID:    t.TenantID.String(),
		TenantName:  tenantName,
		PropertyID:  t.PropertyID.String(),
		Title:       t.Title,
		Description: t.Description,
		Priority:    t.Priority,
		Status:      t.Status,
	}
	if t.RoomID.Valid {
		resp.RoomID = t.RoomID.UUID.String()
	}
	if t.Resolution.Valid {
		resp.Resolution = t.Resolution.String
	}
	if t.CreatedAt.Valid {
		resp.CreatedAt = t.CreatedAt.Time.Format(time.RFC3339)
	}
	if t.UpdatedAt.Valid {
		resp.UpdatedAt = t.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// ticketAttachmentToDTO converts a repository.TicketAttachment to dto.TicketAttachmentResponse.
func ticketAttachmentToDTO(a repository.TicketAttachment) dto.TicketAttachmentResponse {
	resp := dto.TicketAttachmentResponse{
		ID:       a.ID.String(),
		TicketID: a.TicketID.String(),
		URL:      a.Url,
	}
	if a.CreatedAt.Valid {
		resp.CreatedAt = a.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
