package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// bodyCap bounds request body size via http.MaxBytesReader. Applied at the
// router root (uploads) and tightened per-handler for JSON endpoints.
func bodyCap(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// capJSONBody tightens the already-capped body to a small budget for the
// unauthenticated / code-submission endpoints an attacker can hammer.
func capJSONBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// ipLimiter is a per-key sliding window used to throttle brute-force
// attempts on unauthenticated endpoints (login/register) by client IP.
type ipLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	window  time.Duration
	allowed int
}

func newIPLimiter(allowed int, window time.Duration) *ipLimiter {
	return &ipLimiter{hits: map[string][]time.Time{}, window: window, allowed: allowed}
}

func (l *ipLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.allowed {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

// loginLimiter: 10 attempts / minute / IP — enough for typos, hostile to
// credential stuffing.
func (s *Server) loginLimiter() gin.HandlerFunc {
	l := newIPLimiter(10, time.Minute)
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(429, gin.H{"error": "too many attempts, slow down"})
			return
		}
		c.Next()
	}
}

// registerLimiter: 5 / minute / IP (invite codes are entropy-backed, this
// just keeps code-guessing and spam registrations cheap to run).
func (s *Server) registerLimiter() gin.HandlerFunc {
	l := newIPLimiter(5, time.Minute)
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(429, gin.H{"error": "too many attempts, slow down"})
			return
		}
		c.Next()
	}
}

// banGate rejects authenticated requests from banned users. Ban is checked
// on every request (not just at token issue) so bans take effect on live
// sessions within the JWT's lifetime. Register/login pass through before
// auth claims exist (the login handler checks the flag itself).
func (s *Server) banGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := auth.CurrentUser(c)
		if claims == nil {
			c.Next()
			return
		}
		user := &model.User{}
		// one primary-key read per request; the users table is small and the
		// index lookup is cheap compared to any handler's queries.
		if err := s.DB.Select("banned").First(user, claims.UserID).Error; err == nil && user.Banned {
			c.AbortWithStatusJSON(403, gin.H{"error": "账号已被封禁，请联系管理员"})
			return
		}
		c.Next()
	}
}
