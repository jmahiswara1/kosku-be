// Package config loads application configuration from environment variables.
// It uses godotenv to load a .env file and viper to read and validate all
// required configuration values.
package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all application configuration values loaded from environment
// variables. All fields are required unless noted otherwise.
type Config struct {
	// Server
	Port string

	// Database
	DatabaseURL string

	// Supabase
	SupabaseURL        string
	SupabaseJWTSecret  string
	SupabaseServiceKey string

	// Sentry (optional — empty string disables Sentry)
	SentryDSN string

	// CORS
	AllowedOrigins []string

	// Email
	ResendAPIKey string
}

// Load reads configuration from a .env file (if present) and then from the
// environment. It returns an error if any required variable is missing.
func Load(envFile string) (*Config, error) {
	// Load .env file — ignore error if the file does not exist so that
	// production environments that rely solely on real env vars still work.
	if envFile == "" {
		envFile = ".env"
	}
	_ = godotenv.Load(envFile)

	v := viper.New()
	v.AutomaticEnv()

	// Bind all expected keys so viper can read them from the environment.
	bindKeys(v)

	// Set defaults for optional values.
	v.SetDefault("PORT", "8080")

	cfg := &Config{
		Port:               v.GetString("PORT"),
		DatabaseURL:        v.GetString("DATABASE_URL"),
		SupabaseURL:        v.GetString("SUPABASE_URL"),
		SupabaseJWTSecret:  v.GetString("SUPABASE_JWT_SECRET"),
		SupabaseServiceKey: v.GetString("SUPABASE_SERVICE_KEY"),
		SentryDSN:          v.GetString("SENTRY_DSN"),
		ResendAPIKey:       v.GetString("RESEND_API_KEY"),
	}

	// Parse ALLOWED_ORIGINS as a comma-separated list.
	rawOrigins := v.GetString("ALLOWED_ORIGINS")
	if rawOrigins != "" {
		for _, o := range strings.Split(rawOrigins, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
			}
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// bindKeys explicitly binds each environment variable key to viper so that
// AutomaticEnv picks them up regardless of case sensitivity on the OS.
func bindKeys(v *viper.Viper) {
	keys := []string{
		"PORT",
		"DATABASE_URL",
		"SUPABASE_URL",
		"SUPABASE_JWT_SECRET",
		"SUPABASE_SERVICE_KEY",
		"SENTRY_DSN",
		"ALLOWED_ORIGINS",
		"RESEND_API_KEY",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
}

// validate checks that all required configuration fields are non-empty.
func (c *Config) validate() error {
	required := map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"SUPABASE_URL":         c.SupabaseURL,
		"SUPABASE_JWT_SECRET":  c.SupabaseJWTSecret,
		"SUPABASE_SERVICE_KEY": c.SupabaseServiceKey,
		"RESEND_API_KEY":       c.ResendAPIKey,
	}

	var missing []string
	for key, val := range required {
		if strings.TrimSpace(val) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return nil
}
