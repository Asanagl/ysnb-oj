// Package handler — contest 报名 (registration): contestants pick their team
// type (official/starred) and optional team name; type is immutable after
// registration (jury can override via flags). Unregistered users cannot
// submit into a registration-required contest while it runs.
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

const (
	TeamOfficial = "official"
	TeamStarred  = "starred"
)

type registrationView struct {
	ID        uint      `json:"id"`
	ContestID uint      `json:"contest_id"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	TeamName  string    `json:"team_name"`
	TeamType  string    `json:"team_type"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) registrationView(r model.ContestRegistration) registrationView {
	user := &model.User{}
	name := ""
	if err := s.DB.Select("username, nickname").First(user, r.UserID).Error; err == nil {
		name = user.Nickname
		if name == "" {
			name = user.Username
		}
	}
	return registrationView{
		ID: r.ID, ContestID: r.ContestID, UserID: r.UserID,
		Username: name, TeamName: r.TeamName, TeamType: r.TeamType,
		CreatedAt: r.CreatedAt,
	}
}

// register creates (or confirms) the caller's registration. Team type is
// fixed once registered; re-calling with the same payload just returns the
// existing entry.
func (s *Server) registerContest(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	claims := auth.CurrentUser(c)
	if time.Now().After(contest.EndTime) {
		c.JSON(400, gin.H{"error": "比赛已结束，无需报名（补题无需报名）"})
		return
	}
	var req struct {
		TeamName string `json:"team_name" binding:"max=100"`
		TeamType string `json:"team_type" binding:"required"`
		// 组队赛: the captain registers the whole team by id; members are
		// enrolled as rows sharing the same TeamID.
		TeamID uint `json:"team_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if req.TeamType != TeamOfficial && req.TeamType != TeamStarred {
		c.JSON(400, gin.H{"error": "team_type must be official or starred"})
		return
	}
	// 组队赛 registration: only the captain registers; the team is enrolled
	// as one standing row via TeamID.
	if contest.TeamMode {
		if req.TeamID == 0 {
			c.JSON(400, gin.H{"error": "组队赛需要指定 team_id 报名"})
			return
		}
		team := &model.Team{}
		if err := s.DB.First(team, req.TeamID).Error; err != nil {
			c.JSON(404, gin.H{"error": "team not found"})
			return
		}
		if team.CaptainID != claims.UserID {
			c.JSON(403, gin.H{"error": "仅队长可报名"})
			return
		}
		// already registered under another team?
		var existingRegs int64
		s.DB.Model(&model.ContestRegistration{}).
			Where("contest_id = ? AND team_id = ?", contest.ID, req.TeamID).Count(&existingRegs)
		if existingRegs > 0 {
			c.JSON(200, gin.H{"ok": true, "already": true, "team_id": req.TeamID})
			return
		}
		var members []model.TeamMember
		s.DB.Where("team_id = ?", team.ID).Order("id").Find(&members)
		if len(members) == 0 {
			c.JSON(400, gin.H{"error": "队伍没有成员"})
			return
		}
		if contest.TeamCapacity > 0 && len(members) > contest.TeamCapacity {
			c.JSON(400, gin.H{"error": "队伍人数超过比赛上限（" + strconv.Itoa(contest.TeamCapacity) + " 人）"})
			return
		}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			for _, m := range members {
				reg := &model.ContestRegistration{
					ContestID: contest.ID, UserID: m.UserID,
					TeamName: team.Name, TeamType: req.TeamType, TeamID: team.ID,
				}
				if err := tx.Create(reg).Error; err != nil {
					// a member already registered individually — reject so the
					// team never half-competes against its own member
					return err
				}
				if req.TeamType == TeamStarred {
					flag := &model.ContestUserFlag{ContestID: contest.ID, UserID: m.UserID}
					if err := tx.Where("contest_id = ? AND user_id = ?", contest.ID, m.UserID).
						Assign(model.ContestUserFlag{Starred: true}).FirstOrCreate(flag).Error; err != nil {
						return err
					}
					if err := tx.Model(flag).Update("starred", true).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil && isUniqueViolation(err) {
			c.JSON(400, gin.H{"error": "有队员已单独报名本比赛，请先让其取消报名"})
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "register failed"})
			return
		}
		c.JSON(200, gin.H{"ok": true, "team_id": team.ID, "members": len(members)})
		return
	}
	existing := &model.ContestRegistration{}
	if err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
		First(existing).Error; err == nil {
		c.JSON(200, s.registrationView(*existing))
		return
	}
	reg := &model.ContestRegistration{
		ContestID: contest.ID, UserID: claims.UserID,
		TeamName: req.TeamName, TeamType: req.TeamType,
	}
	if err := s.DB.Create(reg).Error; err != nil {
		// concurrent duplicate: the unique index caught it — return the
		// existing entry instead of a 500
		existing := &model.ContestRegistration{}
		if err2 := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
			First(existing).Error; err2 == nil {
			c.JSON(200, s.registrationView(*existing))
			return
		}
		c.JSON(500, gin.H{"error": "register failed"})
		return
	}
	// starred registrations drive the standings ★ directly (jury can still
	// override via setUserFlags).
	if req.TeamType == TeamStarred {
		flag := &model.ContestUserFlag{ContestID: contest.ID, UserID: claims.UserID}
		s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
			Assign(model.ContestUserFlag{Starred: true}).
			FirstOrCreate(flag)
		s.DB.Model(flag).Update("starred", true)
	}
	c.JSON(200, s.registrationView(*reg))
}

// myRegistration: null when not registered.
func (s *Server) myRegistration(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	claims := auth.CurrentUser(c)
	reg := &model.ContestRegistration{}
	if err := s.DB.Where("contest_id = ? AND user_id = ?", contest.ID, claims.UserID).
		First(reg).Error; err != nil {
		c.JSON(200, gin.H{"registration": nil})
		return
	}
	c.JSON(200, gin.H{"registration": s.registrationView(*reg)})
}

// listRegistrations (jury): the entry list for roll-call/滚榜 management.
func (s *Server) listRegistrations(c *gin.Context) {
	contest, ok := s.judgeGuard(c)
	if !ok {
		return
	}
	var regs []model.ContestRegistration
	s.DB.Where("contest_id = ?", contest.ID).Order("id").Find(&regs)
	out := make([]registrationView, 0, len(regs))
	for _, r := range regs {
		out = append(out, s.registrationView(r))
	}
	c.JSON(200, out)
}

// registrationRequired reports whether the contest demands a registration
// before in-contest submissions. nil (legacy rows) means not required.
func registrationRequired(contest *model.Contest) bool {
	return contest.RequireRegistration != nil && *contest.RequireRegistration
}

// freezePoint returns the instant from which submissions are masked while
// frozen, or nil when the contest is not currently frozen. Precedence:
// automatic FreezeTime if set, otherwise the manual-freeze timestamp; a
// NoFreeze contest never freezes.
func freezePoint(contest *model.Contest, now time.Time) *time.Time {
	if !contest.FreezeEnabled || now.After(contest.EndTime) {
		return nil
	}
	if contest.ManualFrozen && contest.ManualFrozenAt != nil {
		return contest.ManualFrozenAt
	}
	if contest.FreezeTime != nil && now.After(*contest.FreezeTime) {
		return contest.FreezeTime
	}
	return nil
}
