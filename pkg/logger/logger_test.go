package logger_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kosku/backend/pkg/logger"
)

func TestInit_WritesJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(logger.Options{
		Level:  logger.InfoLevel,
		Pretty: false,
		Output: &buf,
	})

	logger.Info("test message")

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output, got empty string")
	}

	// Verify the output is valid JSON.
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\noutput: %s", err, output)
	}

	if entry["message"] != "test message" {
		t.Errorf("expected message=%q, got %v", "test message", entry["message"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected 'time' field in log entry")
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
}

func TestInit_RespectsLogLevel(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(logger.Options{
		Level:  logger.WarnLevel,
		Pretty: false,
		Output: &buf,
	})

	// Debug and Info should be suppressed.
	logger.Debug("debug message")
	logger.Info("info message")

	if buf.Len() > 0 {
		t.Errorf("expected no output for debug/info at warn level, got: %s", buf.String())
	}

	// Warn should appear.
	logger.Warn("warn message")
	if buf.Len() == 0 {
		t.Error("expected warn message to be logged")
	}
}

func TestWith_AddsFields(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(logger.Options{
		Level:  logger.InfoLevel,
		Pretty: false,
		Output: &buf,
	})

	contextLogger := logger.With("request_id", "abc-123")
	contextLogger.Info().Msg("request handled")

	output := buf.String()
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v", err)
	}

	if entry["request_id"] != "abc-123" {
		t.Errorf("expected request_id=abc-123, got %v", entry["request_id"])
	}
}

func TestError_IncludesErrorField(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(logger.Options{
		Level:  logger.ErrorLevel,
		Pretty: false,
		Output: &buf,
	})

	testErr := &testError{msg: "something went wrong"}
	logger.Error("operation failed", testErr)

	output := buf.String()
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v", err)
	}

	if entry["error"] != "something went wrong" {
		t.Errorf("expected error field, got %v", entry["error"])
	}
	if entry["message"] != "operation failed" {
		t.Errorf("expected message=operation failed, got %v", entry["message"])
	}
}

// testError is a simple error implementation for testing.
type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
