// Package handler — security hardening (P0/P1 from the open-source security
// review): security response headers and a global write-endpoint rate
// limiter. Kept in one file so the whole hardening layer is auditable.
package handler

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
)

// securityHeaders sets the baseline response headers for every API reply.
// The SPA's CSP is enforced at the nginx layer too (defense in depth);
// 'unsafe-inline' for styles is required by Element Plus runtime injection.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; "+
				"font-src 'self' data:; connect-src 'self' ws: wss:; script-src 'self'")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		c.Next()
	}
}

// userLimiter is a per-user sliding window for write endpoints (same shape
// as submitLimiter; keyed by claims UserID). Separate instances per class
// so题目录入 and题解发帖 don't share one budget.
type userLimiter struct {
	mu      sync.Mutex
	hits    map[uint][]int64 // userID -> window-start unix seconds
	window  int64            // seconds
	allowed int
}

func newUserLimiter(allowed int, windowSec int64) *userLimiter {
	return &userLimiter{hits: map[uint][]int64{}, allowed: allowed, window: windowSec}
}

func (l *userLimiter) allow(uid uint, now int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[uid][:0]
	n := 0
	for _, t := range l.hits[uid] {
		if now-t < l.window {
			recent = append(recent, t)
			n++
		}
	}
	l.hits[uid] = recent
	if n >= l.allowed {
		return false
	}
	l.hits[uid] = append(l.hits[uid], now)
	return true
}

// prune drops stale buckets; called opportunistically every 1024 allows.
func (l *userLimiter) prune(now int64) {
	if len(l.hits) < 4096 {
		return
	}
	for uid, ts := range l.hits {
		recent := ts[:0]
		for _, t := range ts {
			if now-t < l.window {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(l.hits, uid)
		} else {
			l.hits[uid] = recent
		}
	}
}

// userWriteLimiter builds a gin middleware limiting one write-endpoint class
// per authenticated user.
func (s *Server) userWriteLimiter(allowed int, windowSec int64) gin.HandlerFunc {
	return newUserWriteLimiter(allowed, windowSec)
}

// currentUserID resolves the authenticated user id (0 = anonymous; the
// caller groups anonymous traffic under uid 0 so unauthenticated spam still
// gets one shared bucket).
func currentUserID(c *gin.Context) uint {
	if v, ok := c.Get("current_uid"); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

func timeNowSec() int64 { return time.Now().Unix() }

// userWriteLimiter wraps the generic builder with a resolved user id.
func newUserWriteLimiter(allowed int, windowSec int64) gin.HandlerFunc {
	l := newUserLimiter(allowed, windowSec)
	return func(c *gin.Context) {
		claims := auth.CurrentUser(c)
		uid := uint(0)
		if claims != nil {
			uid = claims.UserID
		}
		now := time.Now().Unix()
		l.prune(now)
		if !l.allow(uid, now) {
			c.AbortWithStatusJSON(429, gin.H{"error": "操作过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}

// writeLimiters holds the shared per-class limiter instances so handlers
// registered across files share budgets.
type writeLimiters struct {
	problem  gin.HandlerFunc // create/import/update problems: 10/min
	upload   gin.HandlerFunc // testdata & judge-source uploads: 20/min
	solution gin.HandlerFunc // 题解 writes: 10/min
	social   gin.HandlerFunc // teams/lists/notices: 20/min
	external gin.HandlerFunc // external sync/crawl triggers: 6/min
	contest  gin.HandlerFunc // contest create/update: 10/min
}

func newWriteLimiters() *writeLimiters {
	return &writeLimiters{
		problem:  newUserWriteLimiter(10, 60),
		upload:   newUserWriteLimiter(20, 60),
		solution: newUserWriteLimiter(10, 60),
		social:   newUserWriteLimiter(20, 60),
		external: newUserWriteLimiter(4, 60),
		contest:  newUserWriteLimiter(10, 60),
	}
}