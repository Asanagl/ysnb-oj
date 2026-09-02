// Package handler — super-admin tier: role escalation (grant/revoke admin
// and super_admin) and account bans. Plain admins keep user/profile tools;
// these decisions outrank ordinary administration and live behind
// requireSuperAdmin so no admin can mint another admin or ban anyone.
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// requireSuperAdmin: gin middleware for the super tier. Kept here rather
// than a RequireRole call because the super tier is exactly one role.
func (s *Server) requireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := auth.CurrentUser(c)
		if claims == nil || !auth.IsSuperAdmin(claims.Role) {
			c.AbortWithStatusJSON(403, gin.H{"error": "super admin required"})
			return
		}
		c.Next()
	}
}

// setSuperRole: grant/revoke admin and super_admin. Guards:
//   - nobody edits their own role (the last super must not lock themselves
//     out — same rule as the old setUserRole);
//   - self-demote protection at the system level: the demotion target count
//     of super admins is checked so the last one cannot be revoked by a peer.
func (s *Server) setSuperRole(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	if claims.UserID == id {
		c.JSON(400, gin.H{"error": "不能修改自己的角色"})
		return
	}
	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	switch req.Role {
	case model.RoleSuperAdmin, model.RoleAdmin, model.RoleSetter, model.RoleUser:
	default:
		c.JSON(400, gin.H{"error": "invalid role"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	// demoting an admin/super_admin: keep at least one super_admin alive
	if auth.IsSuperAdmin(user.Role) && !auth.IsSuperAdmin(req.Role) {
		var n int64
		s.DB.Model(&model.User{}).
			Where("role = ? AND id <> ?", model.RoleSuperAdmin, user.ID).
			Count(&n)
		if n == 0 {
			c.JSON(400, gin.H{"error": "系统必须保留至少一名超级管理员"})
			return
		}
	}
	if err := s.DB.Model(user).Update("role", req.Role).Error; err != nil {
		c.JSON(500, gin.H{"error": "update role failed"})
		return
	}
	c.JSON(200, user)
}

// setUserBan (admin tier): ban/unban ordinary users. Admins cannot ban
// users at or above their own tier, and cannot ban themselves.
func (s *Server) setUserBan(c *gin.Context) {
	claims := auth.CurrentUser(c)
	s.banHandler(c, claims, func(target *model.User) bool {
		return target.Role == model.RoleUser || target.Role == model.RoleSetter
	})
}

// superSetUserBan (super tier): ban/unban anyone except super admins and
// themselves — the last super cannot be banned by a peer super.
func (s *Server) superSetUserBan(c *gin.Context) {
	claims := auth.CurrentUser(c)
	s.banHandler(c, claims, func(target *model.User) bool {
		return !auth.IsSuperAdmin(target.Role)
	})
}

func (s *Server) banHandler(c *gin.Context, claims *auth.Claims, mayBan func(*model.User) bool) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	if claims.UserID == id {
		c.JSON(400, gin.H{"error": "不能封禁自己"})
		return
	}
	var req struct {
		Banned *bool `json:"banned" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	if !mayBan(user) {
		c.JSON(403, gin.H{"error": "权限不足，无法封禁该用户"})
		return
	}
	if err := s.DB.Model(user).Update("banned", *req.Banned).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "banned": *req.Banned})
}
