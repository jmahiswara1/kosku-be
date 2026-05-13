// Package middleware provides Gin middleware for the KosKu API server.
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kosku/backend/pkg/logger"
)

// RequestLogger returns a Gin middleware that logs each HTTP request using
// zerolog. It records the HTTP method, path, status code, latency, client IP,
// and any error message set on the Gin context.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process the request.
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if raw != "" {
			path = path + "?" + raw
		}

		event := logger.Logger.Info()
		if status >= 500 {
			event = logger.Logger.Error()
		} else if status >= 400 {
			event = logger.Logger.Warn()
		}

		event.
			Str("method", method).
			Str("path", path).
			Int("status", status).
			Dur("latency", latency).
			Str("ip", clientIP).
			Str("error", errMsg).
			Msg("request")
	}
}
