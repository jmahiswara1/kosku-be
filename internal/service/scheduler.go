package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/email"
	"github.com/kosku/backend/pkg/logger"
)

// SchedulerService handles background scheduled tasks.
type SchedulerService struct {
	queries     *repository.Queries
	emailClient *email.Client
}

// NewSchedulerService creates a new SchedulerService.
func NewSchedulerService(queries *repository.Queries, emailClient *email.Client) *SchedulerService {
	return &SchedulerService{
		queries:     queries,
		emailClient: emailClient,
	}
}

// SendContractExpiryReminders queries contracts expiring within 30 days and
// sends reminder emails to both the owner and the tenant.
func (s *SchedulerService) SendContractExpiryReminders(ctx context.Context) {
	contracts, err := s.queries.ListExpiringContracts(ctx)
	if err != nil {
		logger.Error("scheduler: list expiring contracts", err)
		return
	}

	for _, contract := range contracts {
		s.sendReminderForContract(ctx, contract)
	}
}

// sendReminderForContract sends expiry reminder emails for a single contract.
func (s *SchedulerService) sendReminderForContract(ctx context.Context, contract repository.Contract) {
	endDate := contract.EndDate.Format("2006-01-02")

	// Fetch tenant profile.
	tenantProfile, err := s.queries.GetProfile(ctx, contract.TenantID)
	if err != nil {
		logger.Error(fmt.Sprintf("scheduler: get tenant profile %s", contract.TenantID), err)
		return
	}

	// Fetch property to get owner ID.
	prop, err := s.queries.GetProperty(ctx, contract.PropertyID)
	if err != nil {
		logger.Error(fmt.Sprintf("scheduler: get property %s", contract.PropertyID), err)
		return
	}

	// Fetch owner profile.
	ownerProfile, err := s.queries.GetProfile(ctx, prop.OwnerID)
	if err != nil {
		logger.Error(fmt.Sprintf("scheduler: get owner profile %s", prop.OwnerID), err)
		return
	}

	// Fetch room to get room number.
	room, err := s.queries.GetRoom(ctx, contract.RoomID)
	if err != nil {
		logger.Error(fmt.Sprintf("scheduler: get room %s", contract.RoomID), err)
		return
	}

	// Send reminder to tenant (email not stored in profiles — use phone as fallback).
	// Note: Supabase Auth manages emails; we use the profile name for the email body.
	// In a real deployment, tenant email would come from Supabase Auth.
	// For now, we log the intent and send to owner only.
	tenantName := tenantProfile.FullName
	ownerName := ownerProfile.FullName
	roomNumber := room.Number
	propertyName := prop.Name

	// Send to owner.
	if ownerProfile.Phone.Valid {
		// Owner email would come from Supabase Auth in production.
		// We log the reminder intent here.
		logger.Info(fmt.Sprintf(
			"scheduler: contract expiry reminder — owner=%s tenant=%s room=%s property=%s end_date=%s",
			ownerName, tenantName, roomNumber, propertyName, endDate,
		))
	}

	// Attempt to send email if we have email addresses.
	// In production, emails come from Supabase Auth; here we use a placeholder.
	ownerEmail := fmt.Sprintf("%s@placeholder.kosku.id", prop.OwnerID.String())
	tenantEmail := fmt.Sprintf("%s@placeholder.kosku.id", contract.TenantID.String())

	if err := s.emailClient.SendContractExpiryReminder(
		ownerEmail, ownerName, tenantName, roomNumber, propertyName, endDate,
	); err != nil {
		logger.Error("scheduler: send expiry reminder to owner", err)
	}

	if err := s.emailClient.SendContractExpiryReminder(
		tenantEmail, tenantName, tenantName, roomNumber, propertyName, endDate,
	); err != nil {
		logger.Error("scheduler: send expiry reminder to tenant", err)
	}
}

// RunContractExpiryScheduler starts a background goroutine that runs
// SendContractExpiryReminders once immediately and then every 24 hours.
func RunContractExpiryScheduler(svc *SchedulerService) {
	go func() {
		// Run once immediately on startup.
		ctx := context.Background()
		svc.SendContractExpiryReminders(ctx)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			ctx := context.Background()
			svc.SendContractExpiryReminders(ctx)
		}
	}()
}
