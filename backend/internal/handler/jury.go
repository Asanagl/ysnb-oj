// Package handler — jury operations for contest creators/admins:
// cancel grades (per submission or per user×problem), cheat/star marks,
// scoped rejudge, notices, manual freeze and time adjustment.
package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// canJudgeContest: only the contest creator and admins hold jury power —
// plain setters must not touch someone else's contest.
func canJudgeContest(c *gin.Context, contest *model.Contest) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	return isAdminRole(claims.Role) || contest.CreatedBy == claims.UserID
}

func (s *Server) contestByIDOr404(c *gin.Context) (*model.Contest, bool) {
	return s.contestByID(c)
}

// judgeGuard loads the contest and verifies jury power in one step.
func (s *Server) judgeGuard(c *gin.Context) (*model.Contest, bool) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return nil, false
	}
	if !canJudgeContest(c, contest) {
		c.JSON(403, gin.H{"error": "jury power required (creator/admin)"})
		return nil, false
	}
	return contest, true
}

// submissionInContest loads a submission by the :sid path param and checks
// it belongs to the contest in the path.
func (s *Server) submissionInContest(c *gin.Context, contestID uint, sidParam string) (*model.Submission, bool) {
	sid, ok := paramID(c, "sid")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid submission id"})
		return nil, false
	}
	sub := &model.Submission{}
	if err := s.DB.First(sub, sid).Error; err != nil {
		c.JSON(404, gin.H{"error": "submission not found"})
		return nil, false
	}
	if sub.ContestID == nil || *sub.ContestID != contestID {
		c.JSON(404, gin.H{"error": "submission not in this contest"})
		return nil, false
	}
	return sub, true
}

// setContestSubmissionCancel toggles the jury cancellation flag on one
// submission (two-granularity rule, transparent & reversible).
func (s *Server) setContestSubmissionCancel(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	sub, ok := s.submissionInContest(c, contest.ID, c.Param("sid"))
	if !ok {
		c.JSON(404, gin.H{"error": "submission not found"})
		return
	}
	var req struct {
		Cancelled *bool `json:"cancelled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if err := s.DB.Model(sub).Update("cancelled", *req.Cancelled).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, sub)
}

// cancelUserProblem wipes one user's grade on one problem: every contest
// submission of that user×problem gets the cancelled flag.
func (s *Server) cancelUserProblem(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	pid, okP := paramID(c, "pid")
	if !okU || !okP {
		c.JSON(400, gin.H{"error": "invalid params"})
		return
	}
	var req struct {
		Cancelled *bool `json:"cancelled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	err := s.DB.Model(&model.Submission{}).
		Where("contest_id = ? AND user_id = ? AND problem_id = ?", contest.ID, uid, pid).
		Update("cancelled", *req.Cancelled).Error
	if err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// rejudgeContestProblem re-tests every kept submission of one problem inside
// this contest (manual trigger — jury controls the timing).
func (s *Server) rejudgeContestProblem(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	pid, okP := paramID(c, "pid")
	if !okP {
		c.JSON(400, gin.H{"error": "invalid params"})
		return
	}
	var subs []model.Submission
	if err := s.DB.Where("contest_id = ? AND problem_id = ? AND cancelled = ?",
		contest.ID, pid, false).Find(&subs).Error; err != nil {
		c.JSON(500, gin.H{"error": "query failed"})
		return
	}
	n := s.requeueSubmissions(c.Request.Context(), subs)
	c.JSON(200, gin.H{"ok": true, "requeued": n})
}

// rejudgeContestSubmission re-tests one submission (reverse lever of cancel).
func (s *Server) rejudgeContestSubmission(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	sub, ok := s.submissionInContest(c, contest.ID, c.Param("sid"))
	if !ok {
		c.JSON(404, gin.H{"error": "submission not found"})
		return
	}
	if sub.Cancelled {
		c.JSON(400, gin.H{"error": "submission is cancelled; restore it first"})
		return
	}
	s.requeueSubmissions(c.Request.Context(), []model.Submission{*sub})
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) requeueSubmissions(ctx context.Context, subs []model.Submission) int {
	n := 0
	for i := range subs {
		err := s.DB.Model(&subs[i]).Updates(map[string]any{
			"status": model.SubPending, "judged_at": nil,
			"lease_until": nil, "cases": "", "compile_message": "",
		}).Error
		if err != nil {
			continue
		}
		if err := s.Queue.Push(ctx, uint64(subs[i].ID)); err != nil {
			continue
		}
		n++
	}
	return n
}

// setUserFlags upserts 打星/作弊 marks for one user in one contest.
func (s *Server) setUserFlags(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU || uid == 0 {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		Starred *bool `json:"starred" binding:"required"`
		Cheated *bool `json:"cheated" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	flag := &model.ContestUserFlag{ContestID: contest.ID, UserID: uid}
	err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, uid).
		Assign(model.ContestUserFlag{Starred: *req.Starred, Cheated: *req.Cheated}).
		FirstOrCreate(flag).Error
	if err == nil {
		err = s.DB.Model(flag).Updates(map[string]any{
			"starred": *req.Starred, "cheated": *req.Cheated,
		}).Error
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "update flags failed"})
		return
	}
	c.JSON(200, flag)
}

func (s *Server) listFlags(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var flags []model.ContestUserFlag
	s.DB.Where("contest_id = ?", contest.ID).Order("user_id").Find(&flags)
	// join display names so the jury sees who is flagged without cross-referencing IDs
	var users []model.User
	s.DB.Select("id, username, nickname").Find(&users)
	nameOf := map[uint]string{}
	for _, u := range users {
		nameOf[u.ID] = u.Nickname
		if nameOf[u.ID] == "" {
			nameOf[u.ID] = u.Username
		}
	}
	out := make([]gin.H, 0, len(flags))
	for _, f := range flags {
		out = append(out, gin.H{
			"id": f.ID, "contest_id": f.ContestID, "user_id": f.UserID,
			"username": nameOf[f.UserID], "starred": f.Starred, "cheated": f.Cheated,
			"updated_at": f.UpdatedAt,
		})
	}
	c.JSON(200, out)
}

// listParticipants returns distinct users with at least one submission in
// this contest — the default source of the jury's flag dropdown.
func (s *Server) listParticipants(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var rows []struct {
		UserID uint `gorm:"column:user_id"`
	}
	s.DB.Model(&model.Submission{}).Where("contest_id = ?", contest.ID).
		Distinct("user_id").Scan(&rows)
	if len(rows) == 0 {
		c.JSON(200, []gin.H{})
		return
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	var users []model.User
	s.DB.Select("id, username, nickname").Where("id IN ?", ids).Order("id").Find(&users)
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{"id": u.ID, "username": u.Username, "nickname": u.Nickname})
	}
	c.JSON(200, out)
}

// searchContestUsers searches ALL registered users by id/username/nickname —
// covers pre-contest starring of teams that have not submitted yet. Judge-
// scoped so setters don't need the admin-only user-list permission.
func (s *Server) searchContestUsers(c *gin.Context) {
	// contest is loaded only for the jury-power check; the search itself
	// spans all users (pre-contest starring needs unsubmitted users).
	if _, ok := s.judgeGuard(c); !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	db := s.DB.Select("id, username, nickname").Limit(20)
	if q == "" {
		db = db.Order("id DESC")
	} else if id, err := strconv.Atoi(q); err == nil && id > 0 {
		db = db.Where("id = ? OR username LIKE ? OR nickname LIKE ?",
			id, "%"+q+"%", "%"+q+"%")
	} else {
		db = db.Where("username LIKE ? OR nickname LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	var users []model.User
	db.Find(&users)
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{"id": u.ID, "username": u.Username, "nickname": u.Nickname})
	}
	c.JSON(200, out)
}

// contestNotices: judges publish, participants read. Hidden-contest notices
// must not leak to users who cannot see the contest itself (same gate as
// getContest, cf. BUG-004 class).
func (s *Server) listNotices(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok || !canViewContest(c, contest) {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	var notices []model.ContestNotice
	s.DB.Where("contest_id = ?", contest.ID).Order("id DESC").Limit(50).Find(&notices)
	c.JSON(200, notices)
}

func (s *Server) createNotice(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	notice := &model.ContestNotice{ContestID: contest.ID, Content: req.Content}
	if err := s.DB.Create(notice).Error; err != nil {
		c.JSON(500, gin.H{"error": "create notice failed"})
		return
	}
	c.JSON(200, notice)
}

func (s *Server) deleteNotice(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	nid, okN := paramID(c, "nid")
	if !okN {
		c.JSON(400, gin.H{"error": "invalid notice id"})
		return
	}
	s.DB.Delete(&model.ContestNotice{}, "id = ? AND contest_id = ?", nid, contest.ID)
	c.JSON(200, gin.H{"ok": true})
}

// setContestFreeze toggles the manual scoreboard freeze (jury lever on top
// of the automatic FreezeTime schedule). Contests created with
// NoManualFreeze refuse this lever; the freeze timestamp is recorded so the
// 封榜遮罩 knows exactly which submissions to mask.
func (s *Server) setContestFreeze(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var req struct {
		Frozen *bool `json:"frozen" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if contest.NoManualFreeze {
		c.JSON(400, gin.H{"error": "该比赛创建时已禁止赛时手动封榜"})
		return
	}
	updates := map[string]any{"manual_frozen": *req.Frozen}
	if *req.Frozen {
		now := time.Now()
		updates["manual_frozen_at"] = &now
	} else {
		updates["manual_frozen_at"] = nil
	}
	if err := s.DB.Model(contest).Updates(updates).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "manual_frozen": *req.Frozen})
}

// setContestReveal is the 滚榜揭示控制台: sets how many bottom-ranked rows
// from the true final board are publicly un-masked.
func (s *Server) setContestReveal(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var req struct {
		Count *int `json:"count" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	count := clampInt(*req.Count, 0, 500)
	if err := s.DB.Model(contest).Update("reveal_count", count).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "reveal_count": count})
}

// setContestTime adjusts the window mid-contest (delays, extensions); the
// countdown and freeze schedule follow immediately.
func (s *Server) setContestTime(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var req struct {
		StartTime time.Time `json:"start_time" binding:"required"`
		EndTime   time.Time `json:"end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if !req.EndTime.After(req.StartTime) {
		c.JSON(400, gin.H{"error": "end_time must be after start_time"})
		return
	}
	err := s.DB.Model(contest).Updates(map[string]any{
		"start_time": req.StartTime, "end_time": req.EndTime,
	}).Error
	if err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, contest)
}
