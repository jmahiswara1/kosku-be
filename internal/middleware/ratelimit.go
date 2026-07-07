// Package middleware provides HTTP middleware for the Kosku API.
//
// NOTE: The rate limiter implementation uses in-memory per-IP tracking.
// This works for single-instance deployments but does NOT work across
// multiple instances behind a load balancer. For production multi-instance
// deployments, consider using a Redis-backed rate limiter.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ipLimiter holds a rate limiter and the last time it was accessed, used for
// cleanup of stale entries.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// limiterStore manages per-IP rate limiters with periodic cleanup.
type limiterStore struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	r        rate.Limit
	b        int
}

// newLimiterStore creates a new store. r is the token refill rate (tokens/sec)
// and b is the burst size.
func newLimiterStore(r rate.Limit, b int) *limiterStore {
	s := &limiterStore{
		limiters: make(map[string]*ipLimiter),
		r:        r,
		b:        b,
	}
	// Periodically remove limiters that have not been seen for 5 minutes.
	go s.cleanup(5 * time.Minute)
	return s
}

// get returns the rate limiter for the given IP, creating one if it does not
// exist.
func (s *limiterStore) get(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.limiters[ip]
	if !ok {
		entry = &ipLimiter{
			limiter: rate.NewLimiter(s.r, s.b),
		}
		s.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// cleanup removes entries that have not been accessed within the given TTL.
func (s *limiterStore) cleanup(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for ip, entry := range s.limiters {
			if time.Since(entry.lastSeen) > ttl {
				delete(s.limiters, ip)
			}
		}
		s.mu.Unlock()
	}
}

// rateLimitMiddleware is the shared implementation used by both exported
// middleware constructors.
func rateLimitMiddleware(store *limiterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := store.get(ip)

		if !limiter.Allow() {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Too many requests. Please try again later.",
				},
			})
			return
		}

		c.Next()
	}
}

// GlobalRateLimiter returns a Gin middleware that enforces a global rate limit
// of 100 requests per minute per IP address (burst of 100).
//
// Satisfies: Requirements 11.5, 12 — "100 req/min per IP".
func GlobalRateLimiter() gin.HandlerFunc {
	// 100 req/min = 100/60 tokens per second, burst of 100.
	store := newLimiterStore(rate.Limit(100.0/60.0), 100)
	return rateLimitMiddleware(store)
}

// AuthRateLimiter returns a Gin middleware that enforces a stricter rate limit
// of 10 requests per minute per IP address (burst of 10), intended for
// authentication endpoints.
//
// Satisfies: Requirements 11.5, 12 — "10 req/min auth endpoints".
func AuthRateLimiter() gin.HandlerFunc {
	// 10 req/min = 10/60 tokens per second, burst of 10.
	store := newLimiterStore(rate.Limit(10.0/60.0), 10)
	return rateLimitMiddleware(store)
}
