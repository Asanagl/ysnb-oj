// Package handler — user edit (student_no/nickname), password reset, and
// registration cancel/edit. Admin-scoped except where noted.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// updateUser lets admins fix 学号/昵称 (role changes stay in setUserRole).
func (s *Server) updateUser(c *gin.Context) {
	uid, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, uid).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	var req struct {
		StudentNo *string `json:"student_no" binding:"required,max=32"`
		Nickname  *string `json:"nickname" binding:"required,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if err := s.DB.Model(user).Updates(map[string]any{
		"student_no": *req.StudentNo, "nickname": *req.Nickname,
	}).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, user)
}

// resetUserPassword sets a new password for one user (admin recovery tool).
// The new password is returned once, like CSV import.
func (s *Server) resetUserPassword(c *gin.Context) {
	uid, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		Password string `json:"password" binding:"omitempty,min=6,max=64"`
	}
	_ = c.ShouldBindJSON(&req)
	newPassword := req.Password
	if newPassword == "" {
		newPassword = randomCode(10)
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		c.JSON(500, gin.H{"error": "hash failed"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, uid).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	if err := s.DB.Model(user).Update("password_hash", hash).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "password": newPassword})
}

// cancelRegistration lets a contestant withdraw before the contest ends.
// After the end it is a record — cancel is refused.
func (s *Server) cancelRegistration(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	claims := auth.CurrentUser(c)
	if time.Now().After(contest.EndTime) {
		c.JSON(400, gin.H{"error": "比赛已结束，报名记录不可撤销"})
		return
	}
	reg := &model.ContestRegistration{}
	if err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
		First(reg).Error; err != nil {
		c.JSON(404, gin.H{"error": "you are not registered"})
		return
	}
	if err := s.DB.Delete(reg).Error; err != nil {
		c.JSON(500, gin.H{"error": "cancel failed"})
		return
	}
	// drop the ★ that registration auto-applied (jury marks stay untouched)
	s.DB.Model(&model.ContestUserFlag{}).
		Where("contest_id = ? AND user_id = ? AND cheated = ?", contest.ID, claims.UserID, false).
		Update("starred", false)
	c.JSON(200, gin.H{"ok": true})
}

// editRegistration: change 队伍名; type changes are jury-only (报名制 rule).
func (s *Server) editRegistration(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	claims := auth.CurrentUser(c)
	if time.Now().After(contest.EndTime) {
		c.JSON(400, gin.H{"error": "比赛已结束，报名信息不可修改"})
		return
	}
	reg := &model.ContestRegistration{}
	if err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
		First(reg).Error; err != nil {
		c.JSON(404, gin.H{"error": "you are not registered"})
		return
	}
	var req struct {
		TeamName string `json:"team_name" binding:"max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if err := s.DB.Model(reg).Update("team_name", req.TeamName).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, s.registrationView(*reg))
}

// adminEditRegistration lets the jury change a registration's type/name.
func (s *Server) adminEditRegistration(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	reg := &model.ContestRegistration{}
	if err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, uid).
		First(reg).Error; err != nil {
		c.JSON(404, gin.H{"error": "registration not found"})
		return
	}
	var req struct {
		TeamName string `json:"team_name" binding:"max=100"`
		TeamType string `json:"team_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if req.TeamType != TeamOfficial && req.TeamType != TeamStarred {
		c.JSON(400, gin.H{"error": "team_type must be official or starred"})
		return
	}
	err := s.DB.Model(reg).Updates(map[string]any{
		"team_name": req.TeamName, "team_type": req.TeamType,
	}).Error
	if err == nil {
		err = s.syncStarFlag(contest.ID, uid, req.TeamType == TeamStarred)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, s.registrationView(*reg))
}

// adminDeleteRegistration removes a registration entry (jury tool).
func (s *Server) adminDeleteRegistration(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	if err := s.DB.Delete(&model.ContestRegistration{},
		"contest_id = ? AND user_id = ?", contest.ID, uid).Error; err != nil {
		c.JSON(500, gin.H{"error": "delete failed"})
		return
	}
	_ = s.syncStarFlag(contest.ID, uid, false)
	c.JSON(200, gin.H{"ok": true})
}

// syncStarFlag keeps the standings ★ in step with registration type changes.
func (s *Server) syncStarFlag(contestID, userID uint, starred bool) error {
	flag := &model.ContestUserFlag{}
	err := s.DB.Where("contest_id = ? AND user_id = ?", contestID, userID).
		Assign(model.ContestUserFlag{Starred: starred}).FirstOrCreate(flag).Error
	if err == nil {
		err = s.DB.Model(flag).Update("starred", starred).Error
	}
	return err
}

var _ = gorm.ErrRecordNotFound
