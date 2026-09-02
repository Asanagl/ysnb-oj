// Package handler — external practice sync (刷题统计报表·外部 OJ 接入).
// Users bind their platform accounts (Codeforces / Luogu …); a background
// worker pulls their submission logs through plugin.SubmitLogFetcher on a
// ticker, de-duplicating by (platform, external submission id). Reports
// merge external rows into a dedicated报表 page with merged activity.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/external"
	"github.com/ysnb/oj/internal/plugin"
)

// aliases keep the handler code readable while persistence lives in its own
// package (store→handler cycle avoidance, mirroring internal/public).
type ExternalRecord = external.Record
type ExternalBinding = external.Binding

// myExternalBindings serves GET /external/bindings — the caller's platform
// handles with sync state.
func (s *Server) myExternalBindings(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var rows []ExternalBinding
	s.DB.Where("user_id = ?", claims.UserID).Order("platform").Find(&rows)
	c.JSON(200, gin.H{
		"bindings":  rows,
		"platforms": s.registeredPlatforms(),
	})
}

// upsertExternalBinding creates or updates one platform handle; a full sync
// is triggered inline on first bind (bounded, see syncBinding).
func (s *Server) upsertExternalBinding(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var req struct {
		Platform string `json:"platform" binding:"required"`
		Handle   string `json:"handle" binding:"required,max=128"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if !pluginSubmitFetcherExists(req.Platform) {
		c.JSON(400, gin.H{"error": "不支持的平台: " + req.Platform})
		return
	}
	binding := &ExternalBinding{}
	err := s.DB.Where("user_id = ? AND platform = ?", claims.UserID, req.Platform).
		First(binding).Error
	if err != nil {
		binding = &ExternalBinding{UserID: claims.UserID, Platform: req.Platform, Handle: req.Handle}
		if err := s.DB.Create(binding).Error; err != nil {
			c.JSON(500, gin.H{"error": "bind failed"})
			return
		}
	} else {
		if err := s.DB.Model(binding).Updates(map[string]any{"handle": req.Handle, "last_error": ""}).Error; err != nil {
			c.JSON(500, gin.H{"error": "update failed"})
			return
		}
	}
	// immediate first sync so the UI shows data without waiting a tick
	stored, lastErr := s.syncBinding(binding, true)
	c.JSON(200, gin.H{"binding": binding, "stored": stored, "error": lastErr})
}

// deleteExternalBinding unbinds (fetched records are kept for history until
// the user asks admin for a purge — rebinding restores the view).
func (s *Server) deleteExternalBinding(c *gin.Context) {
	claims := auth.CurrentUser(c)
	platform := c.Param("platform")
	if err := s.DB.Where("user_id = ? AND platform = ?", claims.UserID, platform).
		Delete(&ExternalBinding{}).Error; err != nil {
		c.JSON(500, gin.H{"error": "unbind failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// syncExternalBinding serves POST /external/bindings/:platform/sync — manual
// "立即同步" from the report page.
func (s *Server) syncExternalBinding(c *gin.Context) {
	claims := auth.CurrentUser(c)
	platform := c.Param("platform")
	binding := &ExternalBinding{}
	if err := s.DB.Where("user_id = ? AND platform = ?", claims.UserID, platform).
		First(binding).Error; err != nil {
		c.JSON(404, gin.H{"error": "未绑定该平台"})
		return
	}
	stored, lastErr := s.syncBinding(binding, false)
	c.JSON(200, gin.H{"binding": binding, "stored": stored, "error": lastErr})
}

// externalReport serves GET /external/report/:id — the刷题统计报表: per-platform
// totals, merged 365-day activity buckets and a cross-platform recent-AC
// feed. Readable by any logged-in user (public profile parity).
func (s *Server) externalReport(c *gin.Context) {
	uid, ok := paramID(c, "id")
	if !ok || uid == 0 {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	var bindings []ExternalBinding
	s.DB.Where("user_id = ?", uid).Find(&bindings)
	platforms := make([]string, 0, len(bindings))
	for _, b := range bindings {
		platforms = append(platforms, b.Platform)
	}

	var records []ExternalRecord
	s.DB.Where("user_id = ?", uid).Order("at desc").Find(&records)

	now := time.Now()
	byPlatform := map[string]*platformStat{}
	daily := map[string]*dailyBucket{}
	firstAC := map[string]int64{} // (platform/problem) → first AC unix time

	for i := range records {
		r := &records[i]
		st, ok := byPlatform[r.Platform]
		if !ok {
			st = &platformStat{Platform: r.Platform}
			byPlatform[r.Platform] = st
		}
		st.Submissions++
		if r.Verdict == "AC" {
			st.AC++
			key := r.Platform + "/" + r.ProblemID
			if _, seen := firstAC[key]; !seen {
				firstAC[key] = r.At
				st.Solved++
			}
		}
		key := time.Unix(r.At, 0).Format("2006-01-02")
		b, ok := daily[key]
		if !ok {
			b = &dailyBucket{Date: key}
			daily[key] = b
		}
		b.Submissions++
		if r.Verdict == "AC" {
			b.AC++
		}
	}
	stats := make([]platformStat, 0, len(byPlatform))
	for _, st := range byPlatform {
		stats = append(stats, *st)
	}
	recentAC := make([]gin.H, 0, 20)
	counted := 0
	for i := range records {
		r := &records[i]
		if r.Verdict != "AC" {
			continue
		}
		key := r.Platform + "/" + r.ProblemID
		if firstAC[key] != r.At {
			continue
		}
		recentAC = append(recentAC, gin.H{
			"platform": r.Platform, "problem_id": r.ProblemID,
			"problem_name": r.ProblemName, "at": r.At,
			"external_id": r.ExternalID,
		})
		counted++
		if counted >= 20 {
			break
		}
	}
	c.JSON(200, gin.H{
		"user_id":     uid,
		"platforms":   platforms,
		"by_platform": stats,
		"activity":    seriesSince(now, 365, daily),
		"recent_ac":   recentAC,
		"bindings":    bindings,
	})
}

type platformStat struct {
	Platform    string `json:"platform"`
	Submissions int    `json:"submissions"`
	AC          int    `json:"ac"`
	Solved      int    `json:"solved"` // distinct problems with ≥1 AC
}

// registeredPlatforms lists plugin ids the sync worker can pull.
func (s *Server) registeredPlatforms() []string {
	return plugin.SubmitFetcherNames()
}

// syncDeadline is the freshness bound the report page shows; the hourly
// scanner plus manual syncs keep bindings inside it.
var syncDeadline = time.Hour