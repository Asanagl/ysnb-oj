// Package handler — public user profile with aggregated practice stats:
// daily activity heatmap, 30-day trend, status distribution and per-tag
// strength. Visible to any logged-in user (公开刷题主页).
package handler

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

func jsonUnmarshalTags(raw string, dst *[]string) error {
	return json.Unmarshal([]byte(raw), dst)
}

type dailyBucket struct {
	Date        string `json:"date"`
	Submissions int    `json:"submissions"`
	AC          int    `json:"ac"`
}

type tagStat struct {
	Tag       string `json:"tag"`
	Solved    int    `json:"solved"`
	Attempted int    `json:"attempted"`
}

// platformBucket extends the daily bucket with per-platform counts so the
// frontend heatmap can merge 本站 + any subset of bound external platforms.
type platformBucket struct {
	Date        string            `json:"date"`
	Submissions int               `json:"submissions"`
	AC          int               `json:"ac"`
	ByPlatform  map[string]*pStat `json:"by_platform"`
}

type pStat struct {
	Submissions int `json:"submissions"`
	AC          int `json:"ac"`
}

type userProfile struct {
	User          gin.H            `json:"user"`
	ByStatus      map[string]int64 `json:"by_status"`
	ACProblems    int              `json:"ac_problems"`
	TriedProblems int              `json:"tried_problems"`
	Activity      []dailyBucket    `json:"activity"` // last 365 days
	Trend         []dailyBucket    `json:"trend"`    // last 30 days
	Tags          []tagStat        `json:"tags"`
	// Platforms carries per-platform daily buckets (external practice sync);
	// nil when the user has no external bindings.
	Platforms []platformBucket `json:"platforms,omitempty"`
}

// userProfile aggregates one user's submissions for the public profile page.
func (s *Server) userProfile(c *gin.Context) {
	uid, ok := paramID(c, "id")
	if !ok || uid == 0 {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, uid).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	var subs []model.Submission
	if err := s.DB.Where("user_id = ?", uid).Find(&subs).Error; err != nil {
		c.JSON(500, gin.H{"error": "query failed"})
		return
	}

	now := time.Now()
	byStatus := map[string]int64{}
	acProblems := map[uint]bool{}
	tried := map[uint]bool{}
	daily := map[string]*dailyBucket{}

	bucketFor := func(t time.Time) *dailyBucket {
		key := t.Format("2006-01-02")
		b, ok := daily[key]
		if !ok {
			b = &dailyBucket{Date: key}
			daily[key] = b
		}
		return b
	}

	for i := range subs {
		sub := subs[i]
		tried[sub.ProblemID] = true
		byStatus[sub.Status]++
		if sub.Status == model.SubAC {
			acProblems[sub.ProblemID] = true
			bucketFor(sub.CreatedAt).AC++
		}
		bucketFor(sub.CreatedAt).Submissions++
	}

	activity := seriesSince(now, 365, daily)
	trend := seriesSince(now, 30, daily)
	tags := s.tagStatsFor(tried, acProblems)

	// External-platform overlay (刷题统计): per-platform daily buckets for
	// every bound account of this user; the frontend picks which platforms
	// to merge into the heatmap. Empty when no binding exists.
	platforms := s.externalPlatformActivity(uid, now)

	c.JSON(200, userProfile{
		User: gin.H{
			"id": user.ID, "username": user.Username,
			"nickname": user.Nickname, "role": user.Role,
			"created_at": user.CreatedAt,
		},
		ByStatus:      byStatus,
		ACProblems:    len(acProblems),
		TriedProblems: len(tried),
		Activity:      activity,
		Trend:         trend,
		Tags:          tags,
		Platforms:     platforms,
	})
}

// externalPlatformActivity merges the user's external-platform records into
// the same 365-day bucket shape, keyed by platform name.
func (s *Server) externalPlatformActivity(uid uint, now time.Time) []platformBucket {
	var records []ExternalRecord
	if err := s.DB.Where("user_id = ?", uid).Find(&records).Error; err != nil || len(records) == 0 {
		return nil
	}
	daily := map[string]*platformBucket{}
	bucket := func(at int64) *platformBucket {
		key := time.Unix(at, 0).Format("2006-01-02")
		b, ok := daily[key]
		if !ok {
			b = &platformBucket{Date: key, ByPlatform: map[string]*pStat{}}
			daily[key] = b
		}
		return b
	}
	for i := range records {
		r := &records[i]
		b := bucket(r.At)
		st, ok := b.ByPlatform[r.Platform]
		if !ok {
			st = &pStat{}
			b.ByPlatform[r.Platform] = st
		}
		st.Submissions++
		if r.Verdict == "AC" {
			st.AC++
		}
	}
	return seriesPlatformSince(now, 365, daily)
}

// seriesPlatformSince zero-fills platform buckets like seriesSince does for
// plain daily buckets, so the merged heatmap stays gap-free.
func seriesPlatformSince(now time.Time, days int, daily map[string]*platformBucket) []platformBucket {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := make([]platformBucket, 0, days)
	for i := days - 1; i >= 0; i-- {
		key := today.AddDate(0, 0, -i).Format("2006-01-02")
		if b, ok := daily[key]; ok {
			out = append(out, *b)
			continue
		}
		out = append(out, platformBucket{Date: key, ByPlatform: map[string]*pStat{}})
	}
	return out
}

// seriesSince builds contiguous daily buckets (zero-filled) for the last n
// days ending today, oldest first.
func seriesSince(now time.Time, days int, daily map[string]*dailyBucket) []dailyBucket {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := make([]dailyBucket, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		b, ok := daily[key]
		if !ok {
			out = append(out, dailyBucket{Date: key})
			continue
		}
		out = append(out, *b)
	}
	return out
}

// tagStatsFor resolves the tags of every attempted problem and aggregates
// solved/attempted counts per tag.
func (s *Server) tagStatsFor(tried map[uint]bool, acProblems map[uint]bool) []tagStat {
	if len(tried) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(tried))
	for id := range tried {
		ids = append(ids, id)
	}
	var problems []model.Problem
	if err := s.DB.Select("id, tags").Where("id IN ?", ids).Find(&problems).Error; err != nil {
		return nil
	}
	stats := map[string]*tagStat{}
	for _, p := range problems {
		for _, tag := range decodeTags(p.Tags) {
			st, ok := stats[tag]
			if !ok {
				st = &tagStat{Tag: tag}
				stats[tag] = st
			}
			if acProblems[p.ID] {
				st.Solved++
			}
			st.Attempted++
		}
	}
	out := make([]tagStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Solved > out[j].Solved })
	return out
}

// decodeTags parses the JSON tag column defensively; malformed payloads
// degrade to no tags instead of failing the profile.
func decodeTags(raw string) []string {
	var tags []string
	_ = jsonUnmarshalTags(raw, &tags)
	return tags
}
