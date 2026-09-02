// Package handler — user problem creation (一般用户出题) with review flow:
// any logged-in user creates a draft (hidden, review=pending); admins
// approve → it joins the public bank, or reject with the author able to
// edit and resubmit.
package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

const (
	ReviewDraft    = "draft"
	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"
)

// createUserProblem: any logged-in user creates a private problem pending
// review. Reviewers can adjust everything before approving.
func (s *Server) createUserProblem(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var payload problemPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.normalize() != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	prob := payload.toModel(nil)
	prob.CreatedBy = claims.UserID
	prob.Visibility = model.VisibilityHidden // stays hidden until approved
	prob.ReviewStatus = ReviewPending
	if payload.CheckerSource == "" {
		prob.CheckerSource = ""
	}
	if err := s.DB.Create(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	c.JSON(200, prob)
}

// myProblems: the author's own problems (any review state).
func (s *Server) myProblems(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var problems []model.Problem
	s.DB.Where("created_by = ? AND contest_id IS NULL", claims.UserID).
		Order("id DESC").Limit(100).Find(&problems)
	c.JSON(200, problems)
}

// updateMyProblem: the author edits their own draft/rejected problem; edits
// on rejected problems resubmit them into pending.
func (s *Server) updateMyProblem(c *gin.Context) {
	pid, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid problem id"})
		return
	}
	claims := auth.CurrentUser(c)
	prob := &model.Problem{}
	if err := s.DB.First(prob, pid).Error; err != nil {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if prob.CreatedBy != claims.UserID {
		c.JSON(403, gin.H{"error": "not your problem"})
		return
	}
	if prob.ReviewStatus == ReviewApproved {
		c.JSON(400, gin.H{"error": "已公开题目请在题库管理中修改"})
		return
	}
	// resubmit-only request: a rejected author can send the problem back to
	// the pending queue without changing content (frontend 重投 button).
	var probe struct {
		Resubmit bool `json:"resubmit"`
	}
	if err := c.ShouldBindJSON(&probe); err == nil && probe.Resubmit {
		if prob.ReviewStatus == ReviewRejected {
			if err := s.DB.Model(prob).Update("review_status", ReviewPending).Error; err != nil {
				c.JSON(500, gin.H{"error": "resubmit failed"})
				return
			}
		}
		c.JSON(200, prob)
		return
	}
	var payload problemPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.normalize() != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	payload.toModel(prob)
	prob.Visibility = model.VisibilityHidden
	if prob.ReviewStatus == ReviewRejected {
		prob.ReviewStatus = ReviewPending // resubmit
	}
	if err := s.DB.Save(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "save failed"})
		return
	}
	c.JSON(200, prob)
}

// reviewProblem (admin): approve or reject a pending problem.
func (s *Server) reviewProblem(c *gin.Context) {
	pid, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid problem id"})
		return
	}
	prob := &model.Problem{}
	if err := s.DB.First(prob, pid).Error; err != nil {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	var req struct {
		Action string `json:"action" binding:"required"` // approve | reject
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	switch req.Action {
	case "approve":
		if err := s.DB.Model(prob).Updates(map[string]any{
			"review_status": ReviewApproved,
			"visibility":    model.VisibilityMembers,
		}).Error; err != nil {
			c.JSON(500, gin.H{"error": "approve failed"})
			return
		}
	case "reject":
		if err := s.DB.Model(prob).Update("review_status", ReviewRejected).Error; err != nil {
			c.JSON(500, gin.H{"error": "reject failed"})
			return
		}
	default:
		c.JSON(400, gin.H{"error": "action must be approve or reject"})
		return
	}
	c.JSON(200, prob)
}

// pendingProblems (admin): the review queue.
func (s *Server) pendingProblems(c *gin.Context) {
	var problems []model.Problem
	s.DB.Where("review_status = ?", ReviewPending).Order("id").Limit(100).Find(&problems)
	c.JSON(200, problems)
}

var _ = fmt.Sprintf
var _ = time.Now
