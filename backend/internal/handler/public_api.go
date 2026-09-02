// Package handler — external public API (v1): read-only endpoints for
// bots, dashboards, CLI tools and the cph protocol bridge. Authenticated
// either anonymously (public read) or with an API Key (X-API-Key header)
// for keyed endpoints. Key records are stored hashed; last-used timestamps
// give admins audit ability.
//
// Why a separate route group: the existing /api/v1/* is JWT-scoped. Mixing
// API-key auth into JWT middleware chains is how "public endpoint blocked"
// / "protected endpoint accidentally open" incidents happen.
package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/public"
)

// ApiKeyRecord is an alias to the shared credential model; the handler
// mints and validates keys, the store migrates the table.
type ApiKeyRecord = public.ApiKeyRecord

// generateAPIKey returns (raw, sha256hex). The raw form is "ojk_" + 32 hex
// chars — identifiable and copy-paste safe.
func generateAPIKey() (raw, hash string) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	raw = "ojk_" + hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:])
}

func hashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// apiKeyLimiter is a fixed-window limiter keyed by caller bucket (per key,
// plus one shared anonymous bucket): 60 requests/minute each.
type apiKeyLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	window  time.Duration
	allowed int
}

func newAPIKeyLimiter() *apiKeyLimiter {
	return &apiKeyLimiter{hits: map[string][]time.Time{}, window: time.Minute, allowed: 60}
}

func (l *apiKeyLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.hits[key][:0]
	n := 0
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
			n++
		}
	}
	l.hits[key] = recent
	if n >= l.allowed {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}

// publicAuth records how the request authenticated: anonymous (public read)
// or with a key id (keyed access).
type publicAuth struct {
	Anonymous bool
	KeyID     uint
}

func (a publicAuth) keyBucket() string {
	if a.KeyID > 0 {
		return "k" + strconv.FormatUint(uint64(a.KeyID), 10)
	}
	return "anon"
}

const publicRateLimitMsg = "API 限流：每分钟最多 60 次请求"

// publicAPIAuth resolves X-API-Key (or ?api_key= for header-less tools)
// into the request scope, enforcing the rate limit for anonymous and keyed
// traffic alike. Anonymous callers get identical read access — keys exist
// for audit and future write scopes, not to gate reads.
func (s *Server) publicAPIAuth() gin.HandlerFunc {
	limiter := newAPIKeyLimiter()
	return func(c *gin.Context) {
		raw := c.GetHeader("X-API-Key")
		if raw == "" {
			raw = c.Query("api_key")
		}
		authSc := publicAuth{Anonymous: raw == ""}
		if raw != "" {
			rec := &ApiKeyRecord{}
			if err := s.DB.Where("key_hash = ?", hashAPIKey(raw)).First(rec).Error; err != nil || rec.Revoked {
				c.AbortWithStatusJSON(401, gin.H{"error": "无效或已吊销的 API Key"})
				return
			}
			authSc.KeyID = rec.ID
			now := time.Now()
			s.DB.Model(rec).Update("last_used", &now)
		}
		if !limiter.allow(authSc.keyBucket()) {
			c.AbortWithStatusJSON(429, gin.H{"error": publicRateLimitMsg})
			return
		}
		c.Set("public_auth", authSc)
		c.Next()
	}
}

// currentPublicAuth extracts the resolved auth from the middleware chain.
func currentPublicAuth(c *gin.Context) publicAuth {
	if v, ok := c.Get("public_auth"); ok {
		if a, ok := v.(publicAuth); ok {
			return a
		}
	}
	return publicAuth{}
}

// requireAPIKey rejects anonymous callers — the anti-scrape line for
// endpoints that hand out machine-consumable assets.
func requireAPIKey(c *gin.Context) bool {
	if currentPublicAuth(c).KeyID == 0 {
		c.AbortWithStatusJSON(401, gin.H{"error": "此端点需要 API Key（X-API-Key 头），后台可生成"})
		return false
	}
	return true
}

// --- key management (admin console) ---

type createKeyReq struct {
	Name string `json:"name" binding:"required,max=100"`
}

// createAPIKey mints a new key; the raw value is returned exactly once.
func (s *Server) createAPIKey(c *gin.Context) {
	var req createKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	raw, hash := generateAPIKey()
	claims := auth.CurrentUser(c)
	rec := &ApiKeyRecord{Name: req.Name, KeyHash: hash, CreatedBy: claims.UserID}
	if err := s.DB.Create(rec).Error; err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	rec.Prefix = raw[:10]
	if err := s.DB.Model(rec).Update("prefix", rec.Prefix).Error; err != nil {
		c.JSON(500, gin.H{"error": "update prefix failed"})
		return
	}
	c.JSON(200, gin.H{"id": rec.ID, "name": rec.Name, "key": raw, "prefix": rec.Prefix, "created_at": rec.CreatedAt})
}

func (s *Server) listAPIKeys(c *gin.Context) {
	var keys []ApiKeyRecord
	s.DB.Order("id desc").Limit(200).Find(&keys)
	c.JSON(200, keys)
}

// revokeAPIKey flips the revoked flag (keys are never deleted: audit trail).
func (s *Server) revokeAPIKey(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := s.DB.Model(&ApiKeyRecord{}).Where("id = ?", id).Update("revoked", true).Error; err != nil {
		c.JSON(500, gin.H{"error": "revoke failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// --- public read endpoints ---

// publicUserSummary serves GET /public/users/:id — practice summary, recent
// first-AC list and the 365-day activity heatmap for dashboards and bots.
// :id may be numeric id or username.
func (s *Server) publicUserSummary(c *gin.Context) {
	idParam := c.Param("id")
	user := &model.User{}
	if uid, err := strconv.ParseUint(idParam, 10, 64); err == nil && uid > 0 {
		if err := s.DB.First(user, uint(uid)).Error; err != nil {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
	} else if err := s.DB.Where("username = ?", idParam).First(user).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}

	var subs []model.Submission
	if err := s.DB.Where("user_id = ?", user.ID).Order("created_at asc").Find(&subs).Error; err != nil {
		c.JSON(500, gin.H{"error": "query failed"})
		return
	}

	now := time.Now()
	byStatus := map[string]int64{}
	acProblems := map[uint]bool{}
	tried := map[uint]bool{}
	daily := map[string]*dailyBucket{}
	firstACAt := map[uint]time.Time{}

	for i := range subs {
		sub := subs[i]
		tried[sub.ProblemID] = true
		byStatus[sub.Status]++
		key := sub.CreatedAt.Format("2006-01-02")
		b, ok := daily[key]
		if !ok {
			b = &dailyBucket{Date: key}
			daily[key] = b
		}
		b.Submissions++
		if sub.Status == model.SubAC {
			if _, seen := firstACAt[sub.ProblemID]; !seen {
				firstACAt[sub.ProblemID] = sub.CreatedAt
			}
		}
	}
	for pid := range firstACAt {
		acProblems[pid] = true
	}
	// recent first-AC rows, newest first (re-scan desc for presentation)
	recentAC := make([]gin.H, 0, 20)
	for i := len(subs) - 1; i >= 0 && len(recentAC) < 20; i-- {
		sub := subs[i]
		if sub.Status != model.SubAC || !sub.CreatedAt.Equal(firstACAt[sub.ProblemID]) {
			continue
		}
		recentAC = append(recentAC, gin.H{
			"problem_id":    sub.ProblemID,
			"submission_id": sub.ID,
			"time_ms":       sub.TimeMS,
			"language":      sub.Language,
			"at":            sub.CreatedAt,
		})
	}
	activity := seriesSince(now, 365, daily)

	c.JSON(200, gin.H{
		"user":           gin.H{"id": user.ID, "username": user.Username, "nickname": user.Nickname},
		"by_status":      byStatus,
		"ac_problems":    len(acProblems),
		"tried_problems": len(tried),
		"recent_ac":      recentAC,
		"activity":       activity,
	})
}

// publicProblems serves GET /public/problems — the public problem catalog
// (id, title, limits, judge mode, tags). Hidden and contest-exclusive
// problems are never listed; this is the public face of a training OJ.
func (s *Server) publicProblems(c *gin.Context) {
	q := s.DB.Where("contest_id IS NULL AND visibility <> ?", model.VisibilityHidden).
		Select("id, title, time_limit_ms, mem_limit_mb, judge_mode, tags, samples")
	if kw := c.Query("q"); kw != "" {
		q = q.Where("title LIKE ?", "%"+kw+"%")
	}
	var total int64
	q.Model(&model.Problem{}).Count(&total)
	page, size := pageParams(c)
	if size > 100 {
		size = 100
	}
	var rows []model.Problem
	q.Order("id").Offset((page - 1) * size).Limit(size).Find(&rows)
	items := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		samples := []model.Sample{}
		_ = json.Unmarshal([]byte(p.Samples), &samples)
		items = append(items, gin.H{
			"id": p.ID, "title": p.Title,
			"time_limit_ms": p.TimeLimitMS, "mem_limit_mb": p.MemLimitMB,
			"judge_mode": p.JudgeMode, "tags": decodeTags(p.Tags),
			"sample_count": len(samples),
		})
	}
	c.JSON(200, gin.H{"total": total, "items": items})
}