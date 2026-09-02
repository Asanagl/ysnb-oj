// Package handler — 题解区解锁门禁 (solution-area unlock gate).
//
// Rules (confirmed with the user, 2026-08-30):
//   - hidden by default; unlocked once the user has ANY submission on the
//     problem (CE/practice all count — "先动手，再看别人思路");
//   - during a running contest the solution area of that contest's problems
//     is fully locked for participants (read AND post); unlocks after the
//     contest ends;
//   - exemptions (maintenance/audit): problem author, admins, and that
//     contest's jury — everyone else must satisfy the unlock condition.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// problemContest: the problem belongs to a contest (contest-exclusive copy
// or attached bank problem) — return that contest, ok.
func (s *Server) problemContest(prob *model.Problem) (*model.Contest, bool) {
	if prob.ContestID == nil {
		return nil, false
	}
	return s.contestByIDValue(*prob.ContestID)
}

// solutionUnlocked decides whether the current user may read/post in this
// problem's 题解区. Maintenance exemptions come first; otherwise the user
// needs at least one submission on the problem (any verdict), and during a
// running contest the lock applies regardless of submissions.
func (s *Server) solutionUnlocked(c *gin.Context, prob *model.Problem) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	// maintenance exemptions: problem author/admin, or the contest's jury
	if isAdminRole(claims.Role) {
		return true
	}
	if prob.CreatedBy == claims.UserID {
		return true
	}
	if contest, ok := s.problemContest(prob); ok && canJudgeContest(c, contest) {
		return true
	}

	// contest problems: fully locked while the contest is running
	if contest, ok := s.problemContest(prob); ok {
		now := time.Now()
		if now.After(contest.StartTime) && now.Before(contest.EndTime) {
			return false
		}
	}

	// any submission on this problem unlocks (including practice/CE)
	var n int64
	s.DB.Model(&model.Submission{}).
		Where("user_id = ? AND problem_id = ?", claims.UserID, prob.ID).
		Limit(1).Count(&n)
	return n > 0
}

// solutionGuard: load problem, check visibility AND the unlock gate. Returns
// 403 with a distinct message so the UI can show a locked state instead of a
// generic not-found.
func (s *Server) solutionGuard(c *gin.Context) (*model.Problem, bool) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canSeeProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return nil, false
	}
	if !s.solutionUnlocked(c, prob) {
		c.JSON(403, gin.H{"error": "题解区未解锁：请先提交本题（比赛结束后自动开放）"})
		return nil, false
	}
	return prob, true
}
