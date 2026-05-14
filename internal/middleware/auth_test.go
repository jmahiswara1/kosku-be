// Package middleware_test contains unit tests for the JWT authentication
// middleware. Tests use httptest and the golang-jwt library to generate
// tokens with controlled claims and expiry times.
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kosku/backend/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testJWTSecret = "test-jwt-secret-for-unit-tests"

// makeToken creates a signed HS256 JWT with the given claims and expiry.
// Pass a zero time for exp to omit the expiry claim (no expiry).
func makeToken(t *testing.T, secret string, subject string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": subject,
	}
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

// newAuthRouter builds a minimal Gin router protected by the Auth middleware.
// The single GET /protected route returns 200 on success.
func newAuthRouter(secret string) *gin.Engine {
	r := gin.New()
	r.GET("/protected", middleware.Auth(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// doGet performs a GET /protected request with the given Authorization header
// value (pass empty string to omit the header entirely).
func doGet(r *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Missing token
// ---------------------------------------------------------------------------

// TestAuth_MissingToken verifies that a request without an Authorization
// header is rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_MissingToken(t *testing.T) {
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "") // no Authorization header

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_EmptyBearerToken verifies that "Bearer " with no token value is
// rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_EmptyBearerToken(t *testing.T) {
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer ")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_MalformedHeader verifies that an Authorization header that is not
// in "Bearer <token>" format is rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_MalformedHeader(t *testing.T) {
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Token some-value") // wrong scheme

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Invalid / malformed token
// ---------------------------------------------------------------------------

// TestAuth_InvalidToken verifies that a completely malformed token string is
// rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_InvalidToken(t *testing.T) {
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer this.is.not.a.jwt")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_WrongSecret verifies that a token signed with a different secret is
// rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_WrongSecret(t *testing.T) {
	token := makeToken(t, "wrong-secret", "user-123", time.Now().Add(time.Hour))
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_TamperedToken verifies that a token whose payload has been tampered
// with (signature no longer matches) is rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_TamperedToken(t *testing.T) {
	token := makeToken(t, testJWTSecret, "user-123", time.Now().Add(time.Hour))
	// Flip the last character of the signature to invalidate it.
	tampered := token[:len(token)-1] + "X"
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer "+tampered)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Expired token
// ---------------------------------------------------------------------------

// TestAuth_ExpiredToken verifies that a token whose expiry is in the past is
// rejected with HTTP 401.
//
// Requirements: 12
func TestAuth_ExpiredToken(t *testing.T) {
	// Create a token that expired 1 hour ago.
	token := makeToken(t, testJWTSecret, "user-123", time.Now().Add(-time.Hour))
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Valid token
// ---------------------------------------------------------------------------

// TestAuth_ValidToken verifies that a well-formed, unexpired token signed with
// the correct secret is accepted and the request proceeds to the handler.
//
// Requirements: 12
func TestAuth_ValidToken(t *testing.T) {
	token := makeToken(t, testJWTSecret, "user-abc-123", time.Now().Add(time.Hour))
	r := newAuthRouter(testJWTSecret)
	w := doGet(r, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_SetsUserIDInContext verifies that after a successful auth check the
// user ID from the JWT "sub" claim is available in the Gin context.
//
// Requirements: 12
func TestAuth_SetsUserIDInContext(t *testing.T) {
	const expectedUserID = "user-context-check"
	token := makeToken(t, testJWTSecret, expectedUserID, time.Now().Add(time.Hour))

	r := gin.New()
	r.GET("/protected", middleware.Auth(testJWTSecret), func(c *gin.Context) {
		userID := c.GetString(middleware.ContextKeyUserID)
		if userID != expectedUserID {
			t.Errorf("expected userID=%q in context, got %q", expectedUserID, userID)
		}
		c.JSON(http.StatusOK, gin.H{"user_id": userID})
	})

	w := doGet(r, "Bearer "+token)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestAuth_SetsRoleFromAppMetadata verifies that the role from app_metadata
// is correctly extracted and stored in the Gin context.
//
// Requirements: 12
func TestAuth_SetsRoleFromAppMetadata(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user-role-test",
		"exp": time.Now().Add(time.Hour).Unix(),
		"app_metadata": map[string]interface{}{
			"role": "owner",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	r := gin.New()
	r.GET("/protected", middleware.Auth(testJWTSecret), func(c *gin.Context) {
		role := c.GetString(middleware.ContextKeyRole)
		if role != "owner" {
			t.Errorf("expected role=owner, got %q", role)
		}
		c.JSON(http.StatusOK, gin.H{"role": role})
	})

	w := doGet(r, "Bearer "+signed)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}
