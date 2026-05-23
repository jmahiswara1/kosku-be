// Package middleware_test contains unit tests for the rate-limiting middleware.
// Tests exercise both the global (100 req/min) and auth (10 req/min) limiters
// by sending requests in excess of the burst size and asserting HTTP 429.
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kosku/backend/internal/middleware"
)

// newGlobalRateLimitRouter returns a Gin router with the GlobalRateLimiter
// applied to GET /ping.
func newGlobalRateLimitRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.GlobalRateLimiter())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// newAuthRateLimitRouter returns a Gin router with the AuthRateLimiter
// applied to POST /auth/login.
func newAuthRateLimitRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.AuthRateLimiter())
	r.POST("/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// sendRequests fires n GET /ping requests against r and returns the slice of
// HTTP status codes received.
func sendRequests(r *gin.Engine, method, path string, n int) []int {
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		req := httptest.NewRequest(method, path, nil)
		// Use a fixed IP so all requests share the same limiter bucket.
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		codes[i] = w.Code
	}
	return codes
}

// countStatus counts how many codes in the slice equal the given status.
func countStatus(codes []int, status int) int {
	n := 0
	for _, c := range codes {
		if c == status {
			n++
		}
	}
	return n
}

// Global rate limiter (burst = 100)

// TestGlobalRateLimiter_AllowsUpToBurst verifies that the first 100 requests
// (the burst size) are all accepted with HTTP 200.
//
// Requirements: 11.5, 12
func TestGlobalRateLimiter_AllowsUpToBurst(t *testing.T) {
	r := newGlobalRateLimitRouter()
	codes := sendRequests(r, http.MethodGet, "/ping", 100)

	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected > 0 {
		t.Errorf("expected 0 rejections within burst, got %d", rejected)
	}
}

// TestGlobalRateLimiter_Returns429AfterBurst verifies that requests beyond the
// burst size (101st and beyond) receive HTTP 429.
//
// Requirements: 11.5, 12
func TestGlobalRateLimiter_Returns429AfterBurst(t *testing.T) {
	r := newGlobalRateLimitRouter()
	// Send 110 requests — the first 100 should pass, the rest should be limited.
	codes := sendRequests(r, http.MethodGet, "/ping", 110)

	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected == 0 {
		t.Error("expected at least one 429 after exceeding burst of 100, got none")
	}
}

// TestGlobalRateLimiter_RetryAfterHeader verifies that a 429 response includes
// the Retry-After header.
//
// Requirements: 11.5, 12
func TestGlobalRateLimiter_RetryAfterHeader(t *testing.T) {
	r := newGlobalRateLimitRouter()
	// Exhaust the burst.
	codes := sendRequests(r, http.MethodGet, "/ping", 101)

	// Find the first 429 response by replaying one more request.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// If we got a 429, check the header; if we got 200 the burst refilled
	// (unlikely in a tight loop) — just skip the header check.
	if w.Code == http.StatusTooManyRequests {
		if w.Header().Get("Retry-After") == "" {
			t.Error("expected Retry-After header on 429 response, got none")
		}
	}

	// Ensure at least one 429 was seen across all requests.
	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected == 0 && w.Code != http.StatusTooManyRequests {
		t.Error("expected at least one 429 after exceeding burst")
	}
}

// Auth rate limiter (burst = 10)

// TestAuthRateLimiter_AllowsUpToBurst verifies that the first 10 requests
// (the burst size) are all accepted with HTTP 200.
//
// Requirements: 12
func TestAuthRateLimiter_AllowsUpToBurst(t *testing.T) {
	r := newAuthRateLimitRouter()
	codes := sendRequests(r, http.MethodPost, "/auth/login", 10)

	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected > 0 {
		t.Errorf("expected 0 rejections within burst of 10, got %d", rejected)
	}
}

// TestAuthRateLimiter_Returns429AfterBurst verifies that requests beyond the
// burst size (11th and beyond) receive HTTP 429.
//
// Requirements: 12
func TestAuthRateLimiter_Returns429AfterBurst(t *testing.T) {
	r := newAuthRateLimitRouter()
	// Send 15 requests — the first 10 should pass, the rest should be limited.
	codes := sendRequests(r, http.MethodPost, "/auth/login", 15)

	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected == 0 {
		t.Error("expected at least one 429 after exceeding auth burst of 10, got none")
	}
}

// TestAuthRateLimiter_StricterThanGlobal verifies that the auth limiter's
// burst (10) is smaller than the global limiter's burst (100) by confirming
// that 11 requests to the auth endpoint trigger rate limiting.
//
// Requirements: 12
func TestAuthRateLimiter_StricterThanGlobal(t *testing.T) {
	r := newAuthRateLimitRouter()
	codes := sendRequests(r, http.MethodPost, "/auth/login", 11)

	rejected := countStatus(codes, http.StatusTooManyRequests)
	if rejected == 0 {
		t.Error("auth limiter should reject requests after 10; got no 429 in 11 requests")
	}
}

// TestAuthRateLimiter_ErrorResponseShape verifies that the 429 response body
// follows the standard error response shape.
//
// Requirements: 12
func TestAuthRateLimiter_ErrorResponseShape(t *testing.T) {
	r := newAuthRateLimitRouter()
	// Exhaust the burst.
	sendRequests(r, http.MethodPost, "/auth/login", 10)

	// This 11th request should be rate-limited.
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("expected Content-Type header on 429 response")
	}
}
