package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRole returns a Gin middleware that checks whether the authenticated
// user's role (set by the Auth middleware in the ContextKeyRole context key)
// is one of the allowed roles. If the role is not in the allowed list, the
// middleware aborts the request with HTTP 403 Forbidden.
//
// This middleware must be used after the Auth middleware, which is responsible
// for verifying the JWT and populating ContextKeyRole.
//
// Usage:
//
//	ownerRoutes := v1.Group("/properties")
//	ownerRoutes.Use(middleware.RequireRole("owner"))
//
//	tenantRoutes := v1.Group("/me")
//	tenantRoutes.Use(middleware.RequireRole("tenant"))
func RequireRole(roles ...string) gin.HandlerFunc {
	// Build a set for O(1) lookup.
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role := c.GetString(ContextKeyRole)
		if role == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "Access denied: no role assigned",
				},
			})
			return
		}

		if _, ok := allowed[role]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "Access denied: insufficient role",
				},
			})
			return
		}

		c.Next()
	}
}
