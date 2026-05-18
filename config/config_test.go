package config_test

import (
	"os"
	"testing"

	"github.com/kosku/backend/config"
)

// setEnv sets multiple environment variables and returns a cleanup function
// that unsets them all.
func setEnv(t *testing.T, pairs map[string]string) func() {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
	return func() {
		for k := range pairs {
			_ = os.Unsetenv(k)
		}
	}
}

// validEnv returns the minimum set of env vars required for a successful load.
func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":         "postgres://user:pass@localhost:5432/kosku",
		"SUPABASE_URL":         "https://example.supabase.co",
		"SUPABASE_JWT_SECRET":  "super-secret-jwt",
		"SUPABASE_SERVICE_KEY": "service-key-value",
		"RESEND_API_KEY":       "re_test_key",
		"ALLOWED_ORIGINS":      "http://localhost:3000",
		"PORT":                 "8080",
	}
}

func TestLoad_Success(t *testing.T) {
	cleanup := setEnv(t, validEnv())
	defer cleanup()

	cfg, err := config.Load("nonexistent.env") // no .env file — rely on env vars
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected Port=8080, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/kosku" {
		t.Errorf("unexpected DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.SupabaseJWTSecret != "super-secret-jwt" {
		t.Errorf("unexpected SupabaseJWTSecret: %q", cfg.SupabaseJWTSecret)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	env := validEnv()
	delete(env, "PORT") // omit PORT to test default
	cleanup := setEnv(t, env)
	defer cleanup()

	cfg, err := config.Load("nonexistent.env")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default Port=8080, got %q", cfg.Port)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	env := validEnv()
	delete(env, "DATABASE_URL")
	cleanup := setEnv(t, env)
	defer cleanup()

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
}

func TestLoad_MissingSupabaseJWTSecret(t *testing.T) {
	env := validEnv()
	delete(env, "SUPABASE_JWT_SECRET")
	cleanup := setEnv(t, env)
	defer cleanup()

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error for missing SUPABASE_JWT_SECRET, got nil")
	}
}

func TestLoad_MissingResendAPIKey(t *testing.T) {
	env := validEnv()
	delete(env, "RESEND_API_KEY")
	cleanup := setEnv(t, env)
	defer cleanup()

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error for missing RESEND_API_KEY, got nil")
	}
}

func TestLoad_MissingSupabaseURL(t *testing.T) {
	env := validEnv()
	delete(env, "SUPABASE_URL")
	cleanup := setEnv(t, env)
	defer cleanup()

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error for missing SUPABASE_URL, got nil")
	}
}

func TestLoad_MissingSupabaseServiceKey(t *testing.T) {
	env := validEnv()
	delete(env, "SUPABASE_SERVICE_KEY")
	cleanup := setEnv(t, env)
	defer cleanup()

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error for missing SUPABASE_SERVICE_KEY, got nil")
	}
}

func TestLoad_AllMissingRequiredVarsCauseFailure(t *testing.T) {
	// Ensure that when ALL required vars are absent, Load returns an error.
	// This simulates a completely unconfigured environment.
	for _, k := range []string{
		"DATABASE_URL", "SUPABASE_URL", "SUPABASE_JWT_SECRET",
		"SUPABASE_SERVICE_KEY", "RESEND_API_KEY",
	} {
		t.Setenv(k, "")
	}

	_, err := config.Load("nonexistent.env")
	if err == nil {
		t.Fatal("expected error when all required env vars are missing, got nil")
	}
}

func TestLoad_AllowedOriginsParsesSingleOrigin(t *testing.T) {
	env := validEnv()
	env["ALLOWED_ORIGINS"] = "https://app.kosku.id"
	cleanup := setEnv(t, env)
	defer cleanup()

	cfg, err := config.Load("nonexistent.env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "https://app.kosku.id" {
		t.Errorf("unexpected AllowedOrigins: %v", cfg.AllowedOrigins)
	}
}

func TestLoad_AllowedOriginsParsesMultipleOrigins(t *testing.T) {
	env := validEnv()
	env["ALLOWED_ORIGINS"] = "https://app.kosku.id, http://localhost:3000, https://staging.kosku.id"
	cleanup := setEnv(t, env)
	defer cleanup()

	cfg, err := config.Load("nonexistent.env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.AllowedOrigins) != 3 {
		t.Errorf("expected 3 origins, got %d: %v", len(cfg.AllowedOrigins), cfg.AllowedOrigins)
	}
}

func TestLoad_SentryDSNIsOptional(t *testing.T) {
	env := validEnv()
	delete(env, "SENTRY_DSN") // not set
	cleanup := setEnv(t, env)
	defer cleanup()

	cfg, err := config.Load("nonexistent.env")
	if err != nil {
		t.Fatalf("expected no error when SENTRY_DSN is absent, got: %v", err)
	}

	if cfg.SentryDSN != "" {
		t.Errorf("expected empty SentryDSN, got %q", cfg.SentryDSN)
	}
}

func TestLoad_LoadsFromEnvFile(t *testing.T) {
	// Write a temporary .env file.
	f, err := os.CreateTemp(t.TempDir(), "*.env")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = f.Close() }()

	content := `DATABASE_URL=postgres://env-file-user:pass@localhost/db
SUPABASE_URL=https://env-file.supabase.co
SUPABASE_JWT_SECRET=env-file-secret
SUPABASE_SERVICE_KEY=env-file-service-key
RESEND_API_KEY=re_env_file_key
ALLOWED_ORIGINS=http://localhost:3000
PORT=9090
`
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}
	_ = f.Close()

	// Ensure none of these are set in the real environment.
	for _, k := range []string{
		"DATABASE_URL", "SUPABASE_URL", "SUPABASE_JWT_SECRET",
		"SUPABASE_SERVICE_KEY", "RESEND_API_KEY", "ALLOWED_ORIGINS", "PORT",
	} {
		_ = os.Unsetenv(k)
	}

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error loading from env file: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected Port=9090 from env file, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://env-file-user:pass@localhost/db" {
		t.Errorf("unexpected DatabaseURL from env file: %q", cfg.DatabaseURL)
	}
}
