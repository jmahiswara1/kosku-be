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

	tenantName := tenantProfile.FullName
	ownerName := ownerProfile.FullName
	roomNumber := room.Number
	propertyName := prop.Name

	logger.Info(fmt.Sprintf(
		"scheduler: contract expiry reminder — owner=%s tenant=%s room=%s property=%s end_date=%s",
		ownerName, tenantName, roomNumber, propertyName, endDate,
	))

	// Send to owner if email is available.
	if ownerProfile.Email.Valid && ownerProfile.Email.String != "" {
		if err := s.emailClient.SendContractExpiryReminder(
			ownerProfile.Email.String, ownerName, tenantName, roomNumber, propertyName, endDate,
		); err != nil {
			logger.Error("scheduler: send expiry reminder to owner", err)
		}
	}

	// Send to tenant if email is available.
	if tenantProfile.Email.Valid && tenantProfile.Email.String != "" {
		if err := s.emailClient.SendContractExpiryReminder(
			tenantProfile.Email.String, tenantName, tenantName, roomNumber, propertyName, endDate,
		); err != nil {
			logger.Error("scheduler: send expiry reminder to tenant", err)
		}
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
