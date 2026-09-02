// Package handler — global user search for captain pull / jury starring.
// Split out of searchContestUsers: team captains are not jury members but
// need username resolution; the query returns only id/username/nickname
// (never emails or password hashes) and is rate-unconcerned (auth-gated).
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

func (s *Server) searchUsers(c *gin.Context) {
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
