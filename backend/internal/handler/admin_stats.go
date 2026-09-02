// Package handler — admin statistics for the dashboard: overview cards,
// submission trend, peak-judge-concurrency and host load. Setter/admin only.
package handler

import (
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/sysload"
)

type dailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// statsOverview feeds the dashboard cards.
func (s *Server) statsOverview(c *gin.Context) {
	var users, totalSubs, subsToday, acTotal int64
	s.DB.Model(&model.User{}).Count(&users)
	s.DB.Model(&model.Submission{}).Count(&totalSubs)
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
	s.DB.Model(&model.Submission{}).Where("created_at >= ?", todayStart).Count(&subsToday)
	s.DB.Model(&model.Submission{}).Where("status = ?", model.SubAC).Count(&acTotal)

	queueLen, _ := s.Queue.Len(c.Request.Context())
	var daemons, daemonsOnline int64
	s.DB.Model(&model.JudgeDaemon{}).Count(&daemons)
	s.DB.Model(&model.JudgeDaemon{}).Where("status = ?", model.DaemonOnline).Count(&daemonsOnline)

	var contests int64
	s.DB.Model(&model.Contest{}).Count(&contests)
	var problems int64
	s.DB.Model(&model.Problem{}).Where("contest_id IS NULL").Count(&problems)

	c.JSON(200, gin.H{
		"users": users, "submissions_total": totalSubs,
		"submissions_today": subsToday, "ac_total": acTotal,
		"queue_length": queueLen,
		"daemons":      daemons, "daemons_online": daemonsOnline,
		"contests": contests, "problems": problems,
		"api_load": sysload.Read(),
	})
}

// statsTrend returns per-day submission counts for the last ?days= window.
func (s *Server) statsTrend(c *gin.Context) {
	days := clampInt(atoiDefault(c.Query("days"), 14), 1, 90)
	now := time.Now()
	var subs []model.Submission
	s.DB.Select("created_at").Where("created_at >= ?",
		now.AddDate(0, 0, -days)).Find(&subs)

	counts := map[string]int64{}
	for _, sub := range subs {
		key := sub.CreatedAt.Format("2006-01-02")
		counts[key]++
	}
	out := make([]dailyCount, 0, days)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for i := days - 1; i >= 0; i-- {
		key := today.AddDate(0, 0, -i).Format("2006-01-02")
		out = append(out, dailyCount{Date: key, Count: counts[key]})
	}
	c.JSON(200, gin.H{"days": out})
}

// statsConcurrency estimates peak in-judge concurrency per hour for the last
// ?hours= window from judged submissions' [created_at, judged_at] spans
// (still-pending work is counted via the live queue depth, not history).
func (s *Server) statsConcurrency(c *gin.Context) {
	hours := clampInt(atoiDefault(c.Query("hours"), 24), 1, 24*7)
	now := time.Now()
	start := now.Add(-time.Duration(hours) * time.Hour)
	var subs []model.Submission
	// why no contest filter: contest submissions are the bulk of judging load
	// during a contest — excluding them would understate peak concurrency.
	s.DB.Select("created_at, judged_at").Where(
		"created_at >= ? AND judged_at IS NOT NULL", start,
	).Find(&subs)

	type bucket struct {
		Time  string `json:"time"`
		Peak  int    `json:"peak"`
		Count int    `json:"count"`
	}
	out := make([]bucket, 0, hours)
	spans := make([]struct {
		from, to time.Time
	}, 0, len(subs))
	for _, sub := range subs {
		to := start
		if sub.JudgedAt != nil {
			to = *sub.JudgedAt
		}
		if to.Before(sub.CreatedAt) {
			to = sub.CreatedAt
		}
		spans = append(spans, struct{ from, to time.Time }{sub.CreatedAt, to})
	}
	for h := 0; h < hours; h++ {
		bStart := start.Add(time.Duration(h) * time.Hour)
		bEnd := bStart.Add(time.Hour)
		peak, total := 0, 0
		for _, sp := range spans {
			if sp.from.Before(bEnd) && sp.to.After(bStart) {
				total++
			}
		}
		// peak within the bucket: sweep the span endpoints inside it
		events := make([]time.Time, 0, len(spans)*2)
		for _, sp := range spans {
			from, to := sp.from, sp.to
			if from.Before(bStart) {
				from = bStart
			}
			if to.After(bEnd) {
				to = bEnd
			}
			if from.Before(to) {
				events = append(events, from, to)
			}
		}
		sort.Slice(events, func(i, j int) bool { return events[i].Before(events[j]) })
		cur := 0
		for _, t := range events {
			// count starts before ends at the same instant
			isStart := true
			for _, sp := range spans {
				cmpFrom, cmpTo := sp.from, sp.to
				if cmpFrom.Before(bStart) {
					cmpFrom = bStart
				}
				if cmpTo.After(bEnd) {
					cmpTo = bEnd
				}
				if cmpFrom.Equal(t) {
					isStart = true
					break
				}
				if cmpTo.Equal(t) {
					isStart = false
					break
				}
			}
			if isStart {
				cur++
				if cur > peak {
					peak = cur
				}
			} else {
				cur--
			}
		}
		out = append(out, bucket{Time: bStart.Format("01-02 15:04"), Peak: peak, Count: total})
	}
	c.JSON(200, gin.H{"hours": out})
}

// statsLoad reports host load for the API box and every daemon.
func (s *Server) statsLoad(c *gin.Context) {
	var daemons []model.JudgeDaemon
	s.DB.Where("status = ?", model.DaemonOnline).Order("name").Find(&daemons)
	c.JSON(200, gin.H{
		"api":   sysload.Read(),
		"judge": daemons,
	})
}

func atoiDefault(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v == 0 {
		return def
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
