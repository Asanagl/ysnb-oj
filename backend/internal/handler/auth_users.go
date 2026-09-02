package handler

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

const (
	maxCodeSize      = 256 << 10
	inviteCodeLength = 8
)

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	user := &model.User{}
	if err := s.DB.Where("username = ?", req.Username).First(user).Error; err != nil {
		c.JSON(401, gin.H{"error": "wrong username or password"})
		return
	}
	// ban is enforced at login AND on every authenticated request path via
	// the banned middleware; the login check gives a clearer message.
	if user.Banned {
		c.JSON(403, gin.H{"error": "账号已被封禁，请联系管理员"})
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(401, gin.H{"error": "wrong username or password"})
		return
	}
	now := time.Now()
	s.DB.Model(user).Update("last_login_at", &now)
	token, err := s.JWT.Issue(user)
	if err != nil {
		c.JSON(500, gin.H{"error": "issue token failed"})
		return
	}
	c.JSON(200, gin.H{"token": token, "user": user})
}

type registerReq struct {
	Username   string `json:"username" binding:"required,min=3,max=32"`
	Password   string `json:"password" binding:"required,min=6,max=64"`
	Nickname   string `json:"nickname" binding:"max=64"`
	StudentNo  string `json:"student_no" binding:"required,max=32"`
	InviteCode string `json:"invite_code" binding:"required"`
}

func (s *Server) register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	code := &model.InvitationCode{}
	if err := s.DB.Where("code = ?", req.InviteCode).First(code).Error; err != nil {
		c.JSON(400, gin.H{"error": "invalid invite code"})
		return
	}
	if code.UsedCount >= code.MaxUses || (code.ExpiresAt != nil && code.ExpiresAt.Before(time.Now())) {
		c.JSON(400, gin.H{"error": "invite code exhausted or expired"})
		return
	}
	if strings.ContainsAny(req.Username, " \t/@") {
		c.JSON(400, gin.H{"error": "username contains forbidden characters"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "hash password failed"})
		return
	}
	user := &model.User{
		Username: req.Username, PasswordHash: hash,
		Nickname: req.Nickname, Role: model.RoleUser,
		StudentNo: req.StudentNo,
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return tx.Model(code).UpdateColumn("used_count", gorm.Expr("used_count + 1")).Error
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err) {
		c.JSON(400, gin.H{"error": "username already taken"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "register failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// isUniqueViolation maps driver-specific duplicate-key errors for the two
// supported backends.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || // sqlite
		strings.Contains(msg, "duplicate key value") // postgres
}

// me returns the authenticated user's own profile; the SPA calls it after
// reload to rehydrate its session.
func (s *Server) me(c *gin.Context) {
	claims := auth.CurrentUser(c)
	if claims == nil {
		c.JSON(401, gin.H{"error": "login required"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, claims.UserID).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	c.JSON(200, user)
}

func (s *Server) listUsers(c *gin.Context) {
	var users []model.User
	q := s.DB.Order("id").Limit(500)
	if kw := c.Query("q"); kw != "" {
		q = q.Where("username LIKE ? OR student_no LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	q.Find(&users)
	c.JSON(200, users)
}

type importUserRow struct {
	Username  string `json:"username"`
	StudentNo string `json:"student_no"`
	Nickname  string `json:"nickname"`
	Password  string `json:"password"` // generated when the CSV omits it
	Err       string `json:"err,omitempty"`
}

// importUsers accepts CSV rows (username,student_no,nickname[,password]) and
// reports per-row results; generated passwords are returned exactly once so
// the admin can distribute them without ever existing in code or config.
func (s *Server) importUsers(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "missing file field"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "open file failed"})
		return
	}
	defer f.Close()
	rows := parseUserCSV(f, 5000)
	results := make([]importUserRow, 0, len(rows))
	created := 0
	for _, row := range rows {
		if row.Err == "" {
			if err := s.createUserRow(row.Username, row.StudentNo, row.Nickname, row.Password); err != nil {
				row.Err = err.Error()
			} else {
				created++
			}
		}
		results = append(results, row)
	}
	c.JSON(200, gin.H{"created": created, "rows": results})
}

func (s *Server) createUserRow(username, studentNo, nickname, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	user := &model.User{
		Username: username, StudentNo: studentNo, Nickname: nickname,
		PasswordHash: hash, Role: model.RoleUser,
	}
	if err := s.DB.Create(user).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("username %s already exists", username)
		}
		return err
	}
	return nil
}

type inviteCodeReq struct {
	MaxUses        int `json:"max_uses"`
	ExpiresInHours int `json:"expires_in_hours"`
}

func (s *Server) createInviteCode(c *gin.Context) {
	var req inviteCodeReq
	_ = c.ShouldBindJSON(&req)
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	claims := auth.CurrentUser(c)
	code := &model.InvitationCode{
		Code:    randomCode(inviteCodeLength),
		MaxUses: req.MaxUses, CreatedBy: claims.UserID,
	}
	if req.ExpiresInHours > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
		code.ExpiresAt = &exp
	}
	if err := s.DB.Create(code).Error; err != nil {
		c.JSON(500, gin.H{"error": "create invite code failed"})
		return
	}
	c.JSON(200, code)
}

func (s *Server) listInviteCodes(c *gin.Context) {
	var codes []model.InvitationCode
	s.DB.Order("id DESC").Limit(200).Find(&codes)
	c.JSON(200, codes)
}

func (s *Server) deleteInviteCode(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid invite code id"})
		return
	}
	s.DB.Delete(&model.InvitationCode{}, id)
	c.JSON(200, gin.H{"ok": true})
}

// setUserRole lets admins move users between user and setter. Admin/
// super_admin grants live in setSuperRole (super_admin only) — a plain
// admin must not be able to mint another admin. Self role changes are
// refused — the last admin must not be able to lock themselves out.
func (s *Server) setUserRole(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	if claims.UserID == id {
		c.JSON(400, gin.H{"error": "cannot change your own role"})
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
	case model.RoleSetter, model.RoleUser:
	default:
		c.JSON(400, gin.H{"error": "admin 角色的授予/收回需超级管理员操作"})
		return
	}
	user := &model.User{}
	if err := s.DB.First(user, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	if auth.IsSuperAdmin(user.Role) || user.Role == model.RoleAdmin {
		c.JSON(403, gin.H{"error": "admin 角色的授予/收回需超级管理员操作"})
		return
	}
	if err := s.DB.Model(user).Update("role", req.Role).Error; err != nil {
		c.JSON(500, gin.H{"error": "update role failed"})
		return
	}
	c.JSON(200, user)
}

func randomCode(n int) string {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	out := make([]byte, n)
	for i := range out {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		out[i] = alphabet[idx.Int64()]
	}
	return string(out)
}
