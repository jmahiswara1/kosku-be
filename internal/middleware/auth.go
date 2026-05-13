package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

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

// Auth returns a Gin middleware that validates a Supabase-issued JWT Bearer
// token. On success it sets the authenticated user's ID and role in the Gin
// context so downstream handlers can read them via c.GetString(ContextKeyUserID)
// and c.GetString(ContextKeyRole).
//
// Returns HTTP 401 for:
//   - Missing Authorization header
//   - Malformed Bearer token
//   - Invalid, expired, or tampered tokens
func Auth(jwtSecret string) gin.HandlerFunc {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// Supabase uses HMAC-SHA256 (HS256) to sign JWTs.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(jwtSecret), nil
	}

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_TOKEN",
					"message": "Authorization header is required",
				},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_TOKEN_FORMAT",
					"message": "Authorization header must be in the format: Bearer <token>",
				},
			})
			return
		}

		tokenStr := parts[1]

		claims := &supabaseClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, keyFunc,
			jwt.WithValidMethods([]string{"HS256"}),
		)
		if err != nil || !token.Valid {
			code := "INVALID_TOKEN"
			message := "Token is invalid"

			if errors.Is(err, jwt.ErrTokenExpired) {
				code = "TOKEN_EXPIRED"
				message = "Token has expired"
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    code,
					"message": message,
				},
			})
			return
		}

		// Extract user ID from the "sub" claim (standard JWT subject).
		userID, err := claims.GetSubject()
		if err != nil || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_TOKEN",
					"message": "Token is missing subject claim",
				},
			})
			return
		}

		// Resolve role: prefer app_metadata.role, fall back to user_metadata.role,
		// then the top-level role claim.
		role := claims.AppMetadata.Role
		if role == "" {
			role = claims.UserMetadata.Role
		}
		if role == "" {
			role = claims.Role
		}

		c.Set(ContextKeyUserID, userID)
		c.Set(ContextKeyRole, role)

		c.Next()
	}
}
