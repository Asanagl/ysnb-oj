// Package handler — 题解区 (problem solutions): Markdown floors with
// replies, official pinning by author/admin, WYSIWYG-driven.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

type solutionView struct {
	ID         uint      `json:"id"`
	ProblemID  uint      `json:"problem_id"`
	UserID     uint      `json:"user_id"`
	Username   string    `json:"username"`
	ParentID   *uint     `json:"parent_id"`
	Title      string    `json:"title"`
	BodyMD     string    `json:"body_md"`
	IsOfficial bool      `json:"is_official"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) solutionRows(c *gin.Context, prob *model.Problem) []solutionView {
	var sols []model.ProblemSolution
	// official floors first (newest first inside each group)
	s.DB.Where("problem_id = ?", prob.ID).
		Order("is_official DESC, id").Find(&sols)
	var users []model.User
	s.DB.Select("id, username, nickname").Find(&users)
	nameOf := map[uint]string{}
	for _, u := range users {
		nameOf[u.ID] = u.Nickname
		if nameOf[u.ID] == "" {
			nameOf[u.ID] = u.Username
		}
	}
	out := make([]solutionView, 0, len(sols))
	for _, sol := range sols {
		out = append(out, solutionView{
			ID: sol.ID, ProblemID: sol.ProblemID, UserID: sol.UserID,
			Username: nameOf[sol.UserID], ParentID: sol.ParentID,
			Title: sol.Title, BodyMD: sol.BodyMD, IsOfficial: sol.IsOfficial,
			CreatedAt: sol.CreatedAt,
		})
	}
	return out
}

// listSolutions: gated by the solution-area unlock rule (solution_gate.go) —
// hidden until the user submits, fully locked during a running contest;
// author/admin/jury exempt.
func (s *Server) listSolutions(c *gin.Context) {
	prob, ok := s.solutionGuard(c)
	if !ok {
		return
	}
	c.JSON(200, gin.H{"solutions": s.solutionRows(c, prob)})
}

// createSolution: same unlock gate as reading — during a running contest
// participants can neither read nor post.
func (s *Server) createSolution(c *gin.Context) {
	prob, ok := s.solutionGuard(c)
	if !ok {
		return
	}
	claims := auth.CurrentUser(c)
	var req struct {
		Title    string `json:"title" binding:"max=200"`
		BodyMD   string `json:"body_md" binding:"required"`
		ParentID *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.BodyMD) > 64<<10 {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	// reply target must belong to the same problem
	if req.ParentID != nil {
		parent := &model.ProblemSolution{}
		if err := s.DB.First(parent, *req.ParentID).Error; err != nil ||
			parent.ProblemID != prob.ID {
			c.JSON(400, gin.H{"error": "invalid parent"})
			return
		}
	}
	sol := &model.ProblemSolution{
		ProblemID: prob.ID, UserID: claims.UserID,
		ParentID: req.ParentID, Title: req.Title, BodyMD: req.BodyMD,
	}
	if err := s.DB.Create(sol).Error; err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "id": sol.ID})
}

func (s *Server) canModerateSolution(c *gin.Context, sol *model.ProblemSolution) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	if isAdminRole(claims.Role) {
		return true
	}
	prob := &model.Problem{}
	if err := s.DB.First(prob, sol.ProblemID).Error; err == nil {
		if s.canManageProblem(c, prob) {
			return true // problem author pins/modes their 题解区
		}
	}
	return sol.UserID == claims.UserID // own posts are editable/deletable
}

// updateSolution: author edits own post; author/admin toggles official pin.
func (s *Server) updateSolution(c *gin.Context) {
	sid, ok := paramID(c, "sid")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid solution id"})
		return
	}
	sol := &model.ProblemSolution{}
	if err := s.DB.First(sol, sid).Error; err != nil {
		c.JSON(404, gin.H{"error": "solution not found"})
		return
	}
	claims := auth.CurrentUser(c)
	if sol.UserID != claims.UserID && !s.canModerateSolution(c, sol) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var req struct {
		Title      *string `json:"title" binding:"max=200"`
		BodyMD     *string `json:"body_md" binding:"required,max=65536"`
		IsOfficial *bool   `json:"is_official"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	updates := map[string]any{"title": *req.Title, "body_md": *req.BodyMD}
	if req.IsOfficial != nil {
		// only problem author/admin may flip the official pin
		if isAdminRole(claims.Role) || s.isProblemAuthor(c, sol.ProblemID) {
			updates["is_official"] = *req.IsOfficial
		}
	}
	if err := s.DB.Model(sol).Updates(updates).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, sol)
}

func (s *Server) isProblemAuthor(c *gin.Context, problemID uint) bool {
	prob := &model.Problem{}
	if err := s.DB.First(prob, problemID).Error; err != nil {
		return false
	}
	return s.canManageProblem(c, prob)
}

// deleteSolution: author or moderator removes one floor.
func (s *Server) deleteSolution(c *gin.Context) {
	sid, ok := paramID(c, "sid")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid solution id"})
		return
	}
	sol := &model.ProblemSolution{}
	if err := s.DB.First(sol, sid).Error; err != nil {
		c.JSON(404, gin.H{"error": "solution not found"})
		return
	}
	if !s.canModerateSolution(c, sol) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	if err := s.DB.Delete(sol).Error; err != nil {
		c.JSON(500, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
