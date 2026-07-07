package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/email"
)

// AnnouncementService handles business logic for announcements.
type AnnouncementService struct {
	queries     *repository.Queries
	emailClient *email.Client
}

// NewAnnouncementService creates a new AnnouncementService.
func NewAnnouncementService(queries *repository.Queries, emailClient *email.Client) *AnnouncementService {
	return &AnnouncementService{
		queries:     queries,
		emailClient: emailClient,
	}
}

// CreateAnnouncement inserts an announcement row, fans out notification rows to
// targeted tenants, and optionally sends email via Resend.
//
// Targeting logic:
//   - If req.TenantIDs is non-empty: fan out only to those specific tenants.
//   - Else if req.PropertyID is set: fan out to all active tenants in that property.
//   - Else: fan out to all active tenants across all of the owner's properties.
func (s *AnnouncementService) CreateAnnouncement(ctx context.Context, ownerID uuid.UUID, req dto.CreateAnnouncementRequest) (dto.AnnouncementResponse, error) {
	// Build optional property_id.
	var propertyIDArg uuid.NullUUID
	if req.PropertyID != "" {
		pid, err := uuid.Parse(req.PropertyID)
		if err != nil {
			return dto.AnnouncementResponse{}, fmt.Errorf("create announcement: invalid property_id: %w", err)
		}
		propertyIDArg = uuid.NullUUID{UUID: pid, Valid: true}
	}

	// Insert the announcement row.
	ann, err := s.queries.CreateAnnouncement(ctx, repository.CreateAnnouncementParams{
		OwnerID:    ownerID,
		PropertyID: propertyIDArg,
		Title:      req.Title,
		Body:       req.Body,
		SendEmail:  sql.NullBool{Bool: req.SendEmail, Valid: true},
	})
	if err != nil {
		return dto.AnnouncementResponse{}, fmt.Errorf("create announcement: insert: %w", err)
	}

	// Resolve the list of target tenants.
	type tenantInfo struct {
		id       uuid.UUID
		fullName string
		email    string
	}
	var targets []tenantInfo

	if len(req.TenantIDs) > 0 {
		// Individual targeting — use the provided tenant IDs.
		for _, tidStr := range req.TenantIDs {
			tid, err := uuid.Parse(tidStr)
			if err != nil {
				continue // skip invalid UUIDs
			}
			profile, err := s.queries.GetProfile(ctx, tid)
			if err != nil {
				continue // skip tenants that can't be resolved
			}
			email := ""
			if profile.Email.Valid {
				email = profile.Email.String
			}
			targets = append(targets, tenantInfo{id: tid, fullName: profile.FullName, email: email})
		}
	} else if propertyIDArg.Valid {
		// Property-scoped targeting.
		rows, err := s.queries.GetActiveTenantsByProperty(ctx, uuid.NullUUID{UUID: propertyIDArg.UUID, Valid: true})
		if err != nil {
			return dto.AnnouncementResponse{}, fmt.Errorf("create announcement: get tenants by property: %w", err)
		}
		for _, r := range rows {
			email := ""
			if r.Email.Valid {
				email = r.Email.String
			}
			targets = append(targets, tenantInfo{id: r.ID, fullName: r.FullName, email: email})
		}
	} else {
		// All-properties targeting.
		rows, err := s.queries.GetActiveTenantsByOwner(ctx, ownerID)
		if err != nil {
			return dto.AnnouncementResponse{}, fmt.Errorf("create announcement: get tenants by owner: %w", err)
		}
		for _, r := range rows {
			email := ""
			if r.Email.Valid {
				email = r.Email.String
			}
			targets = append(targets, tenantInfo{id: r.ID, fullName: r.FullName, email: email})
		}
	}

	// Fan out: insert a notification row for each targeted tenant.
	notifiedIDs := make([]string, 0, len(targets))
	for _, t := range targets {
		_, err := s.queries.CreateNotification(ctx, repository.CreateNotificationParams{
			UserID:   t.id,
			Type:     "announcement",
			Title:    ann.Title,
			Body:     sql.NullString{String: ann.Body, Valid: true},
			EntityID: uuid.NullUUID{UUID: ann.ID, Valid: true},
		})
		if err != nil {
			// Non-fatal: log and continue so other tenants still receive the notification.
			continue
		}
		notifiedIDs = append(notifiedIDs, t.id.String())

		// Optionally send email — best-effort, non-fatal.
		if req.SendEmail && t.email != "" {
			_ = s.emailClient.SendAnnouncement(
				t.email,
				t.fullName,
				ann.Title,
				ann.Body,
			)
		}
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(
		ownerID,
		"create_announcement",
		"announcement",
		ann.ID,
		map[string]string{
			"announcement_id":  ann.ID.String(),
			"notified_tenants": fmt.Sprintf("%d", len(notifiedIDs)),
		},
	))

	return announcementToDTO(ann, notifiedIDs), nil
}

// announcementToDTO converts a repository.Announcement to dto.AnnouncementResponse.
func announcementToDTO(ann repository.Announcement, notifiedIDs []string) dto.AnnouncementResponse {
	resp := dto.AnnouncementResponse{
		ID:              ann.ID.String(),
		OwnerID:         ann.OwnerID.String(),
		Title:           ann.Title,
		Body:            ann.Body,
		NotifiedTenants: notifiedIDs,
	}
	if ann.PropertyID.Valid {
		resp.PropertyID = ann.PropertyID.UUID.String()
	}
	if ann.SendEmail.Valid {
		resp.SendEmail = ann.SendEmail.Bool
	}
	if ann.CreatedAt.Valid {
		resp.CreatedAt = ann.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
