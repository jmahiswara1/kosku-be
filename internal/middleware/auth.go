package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func ellipticCurveP256() elliptic.Curve {
	return elliptic.P256()
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// Context keys used to store authenticated user information.
const (
	ContextKeyUserID = "user_id"
	ContextKeyRole   = "role"
)

// supabaseClaims represents the JWT claims issued by Supabase Auth.
// Supabase embeds the user's role inside the "app_metadata" claim.
type supabaseClaims struct {
	jwt.RegisteredClaims
	// Role is stored at the top level in some Supabase JWT configurations.
	Role string `json:"role"`
	// AppMetadata contains owner/tenant/staff role set by the application.
	AppMetadata struct {
		Role string `json:"role"`
	} `json:"app_metadata"`
	// UserMetadata may also carry role information.
	UserMetadata struct {
		Role string `json:"role"`
	} `json:"user_metadata"`
}

// jwksCache caches the JWKS keys fetched from Supabase.
var (
	jwksKeys   map[string]*ecdsa.PublicKey
	jwksMu     sync.RWMutex
	jwksExpiry time.Time
)

// fetchJWKS fetches the JWKS from Supabase and caches the keys.
// It is safe for concurrent use.
func fetchJWKS(supabaseURL, apiKey string) (map[string]*ecdsa.PublicKey, error) {
	jwksMu.RLock()
	if jwksKeys != nil && time.Now().Before(jwksExpiry) {
		defer jwksMu.RUnlock()
		return jwksKeys, nil
	}
	jwksMu.RUnlock()

	jwksMu.Lock()
	defer jwksMu.Unlock()

	// Double-check after acquiring write lock
	if jwksKeys != nil && time.Now().Before(jwksExpiry) {
		return jwksKeys, nil
	}

	// Use the standard JWKS endpoint
	jwksURL := supabaseURL + "/auth/v1/.well-known/jwks.json"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create JWKS request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("apikey", apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read JWKS body: %w", err)
	}

	var jwksResp struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}

	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}

	keys := make(map[string]*ecdsa.PublicKey)
	for _, k := range jwksResp.Keys {
		if k.Kty != "EC" {
			continue
		}
		// Decode x and y coordinates from base64url
		xBytes, err := base64URLDecode(k.X)
		if err != nil {
			continue
		}
		yBytes, err := base64URLDecode(k.Y)
		if err != nil {
			continue
		}

		// Build ECDSA public key from coordinates
		pubKey := &ecdsa.PublicKey{
			Curve: ellipticCurveP256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
		keys[k.Kid] = pubKey
	}

	jwksKeys = keys
	jwksExpiry = time.Now().Add(5 * time.Minute)
	return keys, nil
}

// resolveRole extracts the role from Supabase JWT claims.
func resolveRole(claims *supabaseClaims) string {
	if claims.AppMetadata.Role != "" {
		return claims.AppMetadata.Role
	}
	if claims.UserMetadata.Role != "" {
		return claims.UserMetadata.Role
	}
	return claims.Role
}

// verifyToken validates a JWT token string and returns the parsed claims.
// Supports both HS256 (legacy) and ES256 (new Supabase projects).
func verifyToken(tokenStr, jwtSecret, apiKey string) (*supabaseClaims, error) {
	parser := jwt.NewParser()
	unverified, _, err := parser.ParseUnverified(tokenStr, &supabaseClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims := &supabaseClaims{}

	switch unverified.Method.Alg() {
	case "HS256":
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
			return []byte(jwtSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !token.Valid {
			return nil, err
		}

	case "ES256":
		supabaseURL := ""
		iss, _ := unverified.Claims.(*supabaseClaims).GetIssuer()
		if iss != "" {
			supabaseURL = strings.TrimSuffix(iss, "/auth/v1")
		}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			return resolveES256Key(t, supabaseURL, apiKey)
		}, jwt.WithValidMethods([]string{"ES256"}))
		if err != nil || !token.Valid {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported signing method: %s", unverified.Method.Alg())
	}

	return claims, nil
}

// resolveES256Key fetches the ECDSA public key for the given token's kid.
func resolveES256Key(t *jwt.Token, supabaseURL, apiKey string) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
		return nil, errors.New("unexpected signing method")
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("missing kid in token header")
	}
	keys, err := fetchJWKS(supabaseURL, apiKey)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	key, exists := keys[kid]
	if !exists {
		// Force refresh and retry once
		jwksMu.Lock()
		jwksExpiry = time.Time{}
		jwksMu.Unlock()
		keys, err = fetchJWKS(supabaseURL, apiKey)
		if err != nil {
			return nil, fmt.Errorf("refresh JWKS: %w", err)
		}
		key, exists = keys[kid]
		if !exists {
			return nil, fmt.Errorf("key %s not found in JWKS", kid)
		}
	}
	return key, nil
}

// RoleLoader is implemented by any type that can look up a user's role by ID.
// The repository.Queries type satisfies this interface via GetProfile.
type RoleLoader interface {
	GetProfileRole(ctx context.Context, id uuid.UUID) (string, error)
}

// Auth returns a Gin middleware that validates a Supabase-issued JWT Bearer token.
// If roleLoader is non-nil, the user's role is fetched from the database after
// JWT validation (source of truth). If roleLoader is nil, the role is read from
// the JWT claims (app_metadata.role → user_metadata.role → role).
func Auth(jwtSecret, apiKey string, roleLoader ...RoleLoader) gin.HandlerFunc {
	var rl RoleLoader
	if len(roleLoader) > 0 {
		rl = roleLoader[0]
	}
	return func(c *gin.Context) {
		tokenStr, ok := extractBearerToken(c)
		if !ok {
			return
		}

		claims, err := verifyToken(tokenStr, jwtSecret, apiKey)
		if err != nil {
			code, message := "INVALID_TOKEN", "Token is invalid"
			if errors.Is(err, jwt.ErrTokenExpired) {
				code, message = "TOKEN_EXPIRED", "Token has expired"
			}
			fmt.Printf("[AUTH DEBUG] JWT validation error: %v\n", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": code, "message": message},
			})
			return
		}

		userID, err := claims.GetSubject()
		if err != nil || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "INVALID_TOKEN", "message": "Token is missing subject claim"},
			})
			return
		}

		c.Set(ContextKeyUserID, userID)

		// Resolve role: prefer DB lookup (source of truth) over JWT claims.
		if rl != nil {
			uid, parseErr := uuid.Parse(userID)
			if parseErr == nil {
				dbRole, lookupErr := rl.GetProfileRole(c.Request.Context(), uid)
				if lookupErr == nil && dbRole != "" {
					c.Set(ContextKeyRole, dbRole)
					c.Next()
					return
				}
				// Profile not found yet (first-time registration) — fall through
				// to JWT claim so /v1/auth/register can still be called.
			}
		}

		c.Set(ContextKeyRole, resolveRole(claims))
		c.Next()
	}
}

// extractBearerToken reads and validates the Authorization header format.
// Returns the token string and true on success, or aborts the request and returns false.
func extractBearerToken(c *gin.Context) (string, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "MISSING_TOKEN", "message": "Authorization header is required"},
		})
		return "", false
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_TOKEN_FORMAT", "message": "Authorization header must be in the format: Bearer <token>"},
		})
		return "", false
	}
	return parts[1], true
}
