// Package main is the entry point for the KosKu backend HTTP API.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"

	"github.com/kosku/backend/config"
	"github.com/kosku/backend/internal/handler"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/internal/service"
	"github.com/kosku/backend/pkg/email"
	"github.com/kosku/backend/pkg/logger"
	"github.com/kosku/backend/pkg/storage"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	logger.Init(logger.Options{
		Level:  parseLogLevel(os.Getenv("LOG_LEVEL")),
		Pretty: parseBool(os.Getenv("LOG_PRETTY")),
	})

	if mode := os.Getenv("GIN_MODE"); mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Open database via pgx's database/sql driver.
	rawDB, err := openDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}
	defer func() { _ = rawDB.Close() }()

	if err := rawDB.PingContext(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}

	queries := repository.New(rawDB)

	emailClient := email.New(cfg.ResendAPIKey)
	storageClient := storage.New(cfg.SupabaseURL, cfg.SupabaseServiceKey)
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "https://app.kosku.id"
	}

	// Services
	propertySvc := service.NewPropertyService(queries)
	roomSvc := service.NewRoomService(queries, rawDB)
	tenantSvc := service.NewTenantService(queries)
	billingSvc := service.NewBillingService(queries, storageClient, emailClient)
	ticketSvc := service.NewTicketService(queries, storageClient, emailClient)
	messageSvc := service.NewMessageService(queries, emailClient)
	authSvc := service.NewAuthService(queries, emailClient, appURL)
	settingsSvc := service.NewSettingsService(queries, emailClient, appURL)
	notificationSvc := service.NewNotificationService(queries)
	announcementSvc := service.NewAnnouncementService(queries, emailClient)
	contractTemplateSvc := service.NewContractTemplateService(queries, storageClient)
	tenantPortalSvc := service.NewTenantPortalService(queries)
	roomPhotoSvc := service.NewRoomPhotoService(queries, storageClient)

	// Handlers
	propertyHandler := handler.NewPropertyHandler(propertySvc)
	roomHandler := handler.NewRoomHandler(roomSvc)
	tenantHandler := handler.NewTenantHandler(tenantSvc)
	billingHandler := handler.NewBillingHandler(billingSvc)
	ticketHandler := handler.NewTicketHandler(ticketSvc)
	messageHandler := handler.NewMessageHandler(messageSvc)
	authHandler := handler.NewAuthHandler(authSvc)
	settingsHandler := handler.NewSettingsHandler(settingsSvc)
	notificationHandler := handler.NewNotificationHandler(notificationSvc)
	announcementHandler := handler.NewAnnouncementHandler(announcementSvc)
	contractTemplateHandler := handler.NewContractTemplateHandler(contractTemplateSvc)
	tenantPortalHandler := handler.NewTenantPortalHandler(tenantPortalSvc)
	roomPhotoHandler := handler.NewRoomPhotoHandler(roomPhotoSvc)

	// Router setup
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	corsConfig := cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	r.Use(cors.New(corsConfig))

	r.Use(middleware.GlobalRateLimiter())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/v1")

	// Auth routes: register is open; invite/approve/reject require owner.
	authGroup := v1.Group("/auth")
	authGroup.Use(middleware.AuthRateLimiter())
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/invite", middleware.Auth(cfg.SupabaseJWTSecret, cfg.SupabaseServiceKey, queries), middleware.RequireRole("owner"), authHandler.Invite)
		authGroup.POST("/approve/:id", middleware.Auth(cfg.SupabaseJWTSecret, cfg.SupabaseServiceKey, queries), middleware.RequireRole("owner"), authHandler.Approve)
		authGroup.POST("/reject/:id", middleware.Auth(cfg.SupabaseJWTSecret, cfg.SupabaseServiceKey, queries), middleware.RequireRole("owner"), authHandler.Reject)
	}

	// Protected routes
	protected := v1.Group("")
	protected.Use(middleware.Auth(cfg.SupabaseJWTSecret, cfg.SupabaseServiceKey, queries))
	{
		// Properties
		properties := protected.Group("/properties")
		properties.Use(middleware.RequireRole("owner"))
		{
			properties.GET("", propertyHandler.ListProperties)
			properties.POST("", propertyHandler.CreateProperty)
			properties.GET("/:id", propertyHandler.GetProperty)
			properties.PUT("/:id", propertyHandler.UpdateProperty)
			properties.GET("/:id/rooms", roomHandler.ListRooms)
			properties.POST("/:id/rooms", roomHandler.CreateRoom)
			properties.PUT("/:id/layout", roomHandler.UpdateLayout)
		}

		// Rooms
		rooms := protected.Group("/rooms")
		rooms.Use(middleware.RequireRole("owner"))
		{
			rooms.GET("/:id", roomHandler.GetRoom)
			rooms.PUT("/:id", roomHandler.UpdateRoom)
			rooms.GET("/:id/history", roomHandler.GetRoomHistory)
			rooms.POST("/:id/photos", roomPhotoHandler.UploadPhoto)
		}

		// Photos
		protected.DELETE("/photos/:id", middleware.RequireRole("owner"), roomPhotoHandler.DeletePhoto)

		// Tenants
		tenants := protected.Group("/tenants")
		tenants.Use(middleware.RequireRole("owner"))
		{
			tenants.GET("", tenantHandler.ListTenants)
			tenants.GET("/:id", tenantHandler.GetTenant)
			tenants.PUT("/:id", tenantHandler.UpdateTenant)
			tenants.POST("/checkin", tenantHandler.Checkin)
			tenants.POST("/checkout/:id", tenantHandler.Checkout)
			tenants.POST("/:id/blacklist", tenantHandler.Blacklist)
		}

		// Billing
		bills := protected.Group("/bills")
		bills.Use(middleware.RequireRole("owner"))
		{
			bills.POST("/generate", billingHandler.GenerateBills)
			bills.GET("", billingHandler.ListBills)
			bills.GET("/:id", billingHandler.GetBill)
			bills.PUT("/:id/utilities", billingHandler.UpdateUtilities)
		}

		// Payments
		protected.POST("/payments", middleware.RequireRole("tenant"), billingHandler.SubmitPayment)
		protected.PUT("/payments/:id/confirm", middleware.RequireRole("owner"), billingHandler.ConfirmPayment)
		protected.PUT("/payments/:id/reject", middleware.RequireRole("owner"), billingHandler.RejectPayment)

		// Tickets
		tickets := protected.Group("/tickets")
		{
			tickets.POST("", middleware.RequireRole("tenant"), ticketHandler.CreateTicket)
			tickets.GET("", middleware.RequireRole("owner"), ticketHandler.ListTickets)
			tickets.GET("/:id", ticketHandler.GetTicket)
			tickets.PUT("/:id", middleware.RequireRole("owner"), ticketHandler.UpdateTicket)
		}

		// Messages
		messages := protected.Group("/messages")
		{
			messages.GET("", messageHandler.ListConversations)
			messages.GET("/:userId", messageHandler.GetThread)
			messages.POST("", messageHandler.SendMessage)
		}

		// Notifications
		protected.GET("/notifications", notificationHandler.ListNotifications)
		protected.POST("/notifications/read", notificationHandler.MarkAllRead)
		protected.PUT("/notifications/:id/read", notificationHandler.MarkOneRead)

		// Reports
		protected.GET("/reports/financial", middleware.RequireRole("owner"), billingHandler.GetFinancialReport)
		protected.GET("/reports/financial/export", middleware.RequireRole("owner"), billingHandler.ExportFinancialReport)

		// Export
		protected.GET("/export", middleware.RequireRole("owner"), settingsHandler.ExportData)

		// Audit logs
		protected.GET("/audit-logs", middleware.RequireRole("owner"), settingsHandler.ListAuditLogs)

		// Settings
		settings := protected.Group("/settings")
		settings.Use(middleware.RequireRole("owner"))
		{
			settings.GET("", settingsHandler.GetSettings)
			settings.PUT("/profile", settingsHandler.UpdateProfileSettings)
			settings.PUT("/billing", settingsHandler.UpdateBillingSettings)
			settings.GET("/staff", settingsHandler.ListStaff)
			settings.POST("/staff", settingsHandler.AddStaff)
			settings.DELETE("/staff/:id", settingsHandler.RemoveStaff)
		}

		// Expiring contracts (owner only)
		protected.GET("/contracts", middleware.RequireRole("owner"), tenantPortalHandler.ListExpiringContracts)

		// Contract templates
		templates := protected.Group("/contract-templates")
		templates.Use(middleware.RequireRole("owner"))
		{
			templates.GET("", contractTemplateHandler.ListTemplates)
			templates.POST("", contractTemplateHandler.CreateTemplate)
			templates.PUT("/:id", contractTemplateHandler.UpdateTemplate)
			templates.POST("/:id/generate", contractTemplateHandler.GenerateContract)
		}

		// Announcements
		announcements := protected.Group("/announcements")
		announcements.Use(middleware.RequireRole("owner"))
		{
			announcements.POST("", announcementHandler.CreateAnnouncement)
		}

		// Tenant portal
		me := protected.Group("/me")
		me.Use(middleware.RequireRole("tenant"))
		{
			me.GET("/room", tenantPortalHandler.GetMyRoom)
			me.GET("/bills", tenantPortalHandler.ListMyBills)
			me.GET("/bills/:id/receipt", tenantPortalHandler.GetBillReceipt)
			me.GET("/tickets", tenantPortalHandler.ListMyTickets)
			me.POST("/tickets", tenantPortalHandler.CreateMyTicket)
			me.GET("/contracts", tenantPortalHandler.ListMyContracts)
			me.POST("/contracts/renew", tenantPortalHandler.RequestContractRenewal)
		}
	}

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Info().Str("port", cfg.Port).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited")
}

// openDB opens a database connection using the pgx stdlib driver.
func openDB(dsn string) (*sql.DB, error) {
	return stdlib.OpenDB(dsn)
}

// parseLogLevel converts a string log level to a zerolog.Level.
func parseLogLevel(s string) logger.Level {
	switch s {
	case "debug", "DEBUG":
		return logger.DebugLevel
	case "warn", "WARN":
		return logger.WarnLevel
	case "error", "ERROR":
		return logger.ErrorLevel
	default:
		return logger.InfoLevel
	}
}

// parseBool converts a string to a boolean, returning false on error.
func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
