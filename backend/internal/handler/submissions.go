package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// pageParams reads ?page/?size with sane bounds.
func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// submitLimiter is a naive per-user sliding window; the goal is to stop
// accidental floods and grid-search scripts, not determined attackers.
type submitLimiter struct {
	mu      sync.Mutex
	hits    map[uint][]time.Time
	window  time.Duration
	allowed int
}

func newSubmitLimiter() *submitLimiter {
	return &submitLimiter{hits: map[uint][]time.Time{}, window: time.Minute, allowed: 15}
}

func (l *submitLimiter) allow(userID uint) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[userID][:0]
	for _, t := range l.hits[userID] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.allowed {
		l.hits[userID] = recent
		return false
	}
	l.hits[userID] = append(recent, now)
	return true
}

type createSubmissionReq struct {
	ProblemID uint   `json:"problem_id" binding:"required"`
	ContestID *uint  `json:"contest_id"`
	Language  string `json:"language" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

func (s *Server) createSubmission(c *gin.Context) {
	claims := auth.CurrentUser(c)
	if !s.Submits.allow(claims.UserID) {
		c.JSON(429, gin.H{"error": "submission rate limit exceeded, wait a moment"})
		return
	}
	var req createSubmissionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if len(req.Code) > maxCodeSize {
		c.JSON(400, gin.H{"error": "code too large"})
		return
	}
	if _, err := s.Langs.Get(req.Language); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	prob := &model.Problem{}
	if err := s.DB.First(prob, req.ProblemID).Error; err != nil || !s.canSeeProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	// Review flow: the author may see/edit a pending/rejected problem, but no
	// judging may happen on it until an admin approves it. (This deliberately
	// also blocks contest-exclusive copies — those are created approved.)
	if prob.ReviewStatus != ReviewApproved &&
		!(prob.CreatedBy == auth.CurrentUser(c).UserID && prob.ReviewStatus == ReviewDraft) {
		c.JSON(400, gin.H{"error": "题目尚未通过审核，暂不能提交"})
		return
	}
	// Contest-exclusive problems must be submitted inside their contest so
	// the attempt lands on that contest's standings.
	if prob.ContestID != nil && (req.ContestID == nil || *req.ContestID != *prob.ContestID) {
		c.JSON(400, gin.H{"error": "该题为比赛专属题，请从比赛页面提交"})
		return
	}
	practice, err := s.checkContestContext(c, req.ContestID, prob.ID)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sub := &model.Submission{
		UserID: claims.UserID, ProblemID: prob.ID, ContestID: req.ContestID,
		Language: req.Language, CodeSize: len(req.Code), Status: model.SubPending,
		IsPractice: practice,
	}
	if err := s.DB.Create(sub).Error; err != nil {
		c.JSON(500, gin.H{"error": "create submission failed"})
		return
	}
	codePath := filepath.Join(s.Cfg.DataDir, "codes", fmt.Sprint(sub.ID))
	if err := os.MkdirAll(filepath.Dir(codePath), 0o750); err != nil ||
		os.WriteFile(codePath, []byte(req.Code), 0o640) != nil {
		c.JSON(500, gin.H{"error": "store code failed"})
		return
	}
	sub.CodePath = codePath
	s.DB.Model(sub).Update("code_path", codePath)
	if err := s.Queue.Push(c.Request.Context(), uint64(sub.ID)); err != nil {
		c.JSON(500, gin.H{"error": "enqueue judge task failed"})
		return
	}
	c.JSON(200, sub)
}

// checkContestContext validates the contest submission window/registration
// and returns whether this attempt is 赛后补题 (practice): before start is an
// error, during the contest is a counted attempt, after end is practice that
// never touches the frozen standings.
func (s *Server) checkContestContext(c *gin.Context, contestID *uint, problemID uint) (bool, error) {
	if contestID == nil {
		return false, nil
	}
	contest := &model.Contest{}
	if err := s.DB.First(contest, *contestID).Error; err != nil {
		return false, fmt.Errorf("contest not found")
	}
	now := time.Now()
	if now.Before(contest.StartTime) {
		return false, fmt.Errorf("contest has not started")
	}
	practice := now.After(contest.EndTime)
	var linked int64
	s.DB.Model(&model.ContestProblem{}).
		Where("contest_id = ? AND problem_id = ?", *contestID, problemID).Count(&linked)
	if linked == 0 {
		return practice, fmt.Errorf("problem not in this contest")
	}
	// 报名制: in-contest attempts need a registration entry; 赛后补题 bypasses.
	if !practice && registrationRequired(contest) {
		claims := auth.CurrentUser(c)
		var reg int64
		s.DB.Model(&model.ContestRegistration{}).
			Where("contest_id = ? AND user_id = ?", *contestID, claims.UserID).Count(&reg)
		if reg == 0 {
			return practice, fmt.Errorf("请先在比赛页报名后再提交")
		}
	}
	return practice, nil
}

func (s *Server) listSubmissions(c *gin.Context) {
	claims := auth.CurrentUser(c)
	q := s.DB.Model(&model.Submission{})
	if mine := c.Query("mine"); mine == "1" || mine == "true" {
		q = q.Where("user_id = ?", claims.UserID)
	}
	if v := c.Query("problem_id"); v != "" {
		q = q.Where("problem_id = ?", v)
	}
	if v := c.Query("contest_id"); v != "" {
		q = q.Where("contest_id = ?", v)
		// ICPC norm: during a running contest, non-managers see only their
		// own attempts; the full feed opens after the contest ends.
		if cid, err := strconv.ParseUint(v, 10, 64); err == nil {
			contest := &model.Contest{}
			if err := s.DB.First(contest, cid).Error; err == nil &&
				time.Now().Before(contest.EndTime) && !canManageContest(c, contest) {
				q = q.Where("user_id = ?", claims.UserID)
			}
		}
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.Query("user_id"); v != "" && isAdminRole(claims.Role) {
		q = q.Where("user_id = ?", v)
	}
	var total int64
	q.Count(&total)
	page, size := pageParams(c)
	var subs []model.Submission
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&subs)
	// 封榜遮罩: per-row (a page may mix training and contest submissions)
	out := make([]gin.H, 0, len(subs))
	for i := range subs {
		view := s.submissionView(c, &subs[i], false)
		if s.maskSubmission(c, &subs[i]) {
			applyFreezeMask(view)
		}
		out = append(out, view)
	}
	c.JSON(200, gin.H{"total": total, "items": out})
}

func (s *Server) getSubmission(c *gin.Context) {
	sub, ok := s.submissionByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "submission not found"})
		return
	}
	full := s.canViewCode(c, sub)
	view := s.submissionView(c, sub, full)
	if s.maskSubmission(c, sub) {
		applyFreezeMask(view)
	}
	c.JSON(200, view)
}

// maskSubmission reports whether this submission's verdict must be hidden:
// 封榜生效中 + 提交发生在封榜点之后。Judge view (judge_view=1) is honored
// only for the contest's jury (creator/admin).
func (s *Server) maskSubmission(c *gin.Context, sub *model.Submission) bool {
	if sub.ContestID == nil || sub.IsPractice {
		return false
	}
	contest := &model.Contest{}
	if err := s.DB.First(contest, *sub.ContestID).Error; err != nil {
		return false
	}
	fp := freezePoint(contest, time.Now())
	if fp == nil || !sub.CreatedAt.After(*fp) {
		return false
	}
	if c.Query("judge_view") == "1" && canJudgeContest(c, contest) {
		return false
	}
	return true
}

// applyFreezeMask replaces verdict-bearing fields with frozen placeholders;
// the submission row itself stays visible (提交时间/用户/题目 keep showing).
func applyFreezeMask(view gin.H) {
	view["status"] = "SUBMITTED"
	view["cases"] = []model.CaseResult{}
	view["compile_message"] = ""
	view["judged_at"] = nil
	view["time_ms"] = 0
	view["memory_kb"] = 0
}

// canViewCode: own submissions always; admin always; contest submissions of
// others only after the contest ends (standard training-share policy).
func (s *Server) canViewCode(c *gin.Context, sub *model.Submission) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	if isAdminRole(claims.Role) || claims.UserID == sub.UserID {
		return true
	}
	if sub.ContestID != nil {
		contest := &model.Contest{}
		if err := s.DB.First(contest, *sub.ContestID).Error; err == nil {
			return time.Now().After(contest.EndTime)
		}
	}
	return false
}

func (s *Server) submissionView(c *gin.Context, sub *model.Submission, withCode bool) gin.H {
	cases := []model.CaseResult{}
	_ = json.Unmarshal([]byte(sub.Cases), &cases)
	view := gin.H{
		"id": sub.ID, "user_id": sub.UserID, "problem_id": sub.ProblemID,
		"contest_id": sub.ContestID, "language": sub.Language,
		"code_size": sub.CodeSize, "status": sub.Status, "score": sub.Score,
		"time_ms": sub.TimeMS, "memory_kb": sub.MemoryKB,
		"compile_message": sub.CompileMessage, "cases": cases,
		"created_at": sub.CreatedAt, "judged_at": sub.JudgedAt,
	}
	if withCode {
		raw, err := os.ReadFile(sub.CodePath)
		if err == nil {
			view["code"] = string(raw)
		}
	} else if claims := auth.CurrentUser(c); claims != nil && claims.UserID != sub.UserID {
		view["user_id_masked"] = true // keep identity visible, code hidden
	}
	return view
}

func (s *Server) myStats(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var byStatus []struct {
		Status string
		Count  int64
	}
	s.DB.Model(&model.Submission{}).Select("status, count(*) as count").
		Where("user_id = ?", claims.UserID).Group("status").Scan(&byStatus)
	var acProblems int64
	s.DB.Model(&model.Submission{}).Where("user_id = ? AND status = ?",
		claims.UserID, model.SubAC).Distinct("problem_id").Count(&acProblems)
	stats := map[string]int64{}
	for _, row := range byStatus {
		stats[row.Status] = row.Count
	}
	c.JSON(200, gin.H{"by_status": stats, "ac_problems": acProblems})
}
