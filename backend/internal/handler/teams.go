// Package handler — training teams (训练小组): ICPC-style captain + members
// with a configurable capacity (default 3). Join by invite code or captain
// pull; captain manages notices, the shared 题单, and can transfer
// captaincy. Teams are independent of contest registration for now
// (组队赛报名 planned separately).
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

type teamView struct {
	model.Team
	MemberCount int64            `json:"member_count"`
	Captain     string           `json:"captain"`
	Role        string           `json:"role"` // captain | member | none (caller's)
	Members     []teamMemberView `json:"members,omitempty"`
}

type teamMemberView struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	IsCaptain bool   `json:"is_captain"`
}

func (s *Server) teamMembers(teamID uint) []teamMemberView {
	var members []model.TeamMember
	s.DB.Where("team_id = ?", teamID).Order("id").Find(&members)
	var users []model.User
	s.DB.Select("id, username, nickname").Find(&users)
	name := map[uint]model.User{}
	for _, u := range users {
		name[u.ID] = u
	}
	team := &model.Team{}
	captain := uint(0)
	if err := s.DB.Select("captain_id").First(team, teamID).Error; err == nil {
		captain = team.CaptainID
	}
	out := make([]teamMemberView, 0, len(members))
	for _, m := range members {
		u := name[m.UserID]
		out = append(out, teamMemberView{
			UserID: m.UserID, Username: u.Username, Nickname: u.Nickname,
			IsCaptain: m.UserID == captain,
		})
	}
	return out
}

// teamRole: caller's role in the team.
func (s *Server) teamRole(team *model.Team, claims *auth.Claims) string {
	if claims == nil {
		return "none"
	}
	if team.CaptainID == claims.UserID {
		return "captain"
	}
	var n int64
	s.DB.Model(&model.TeamMember{}).Where("team_id = ? AND user_id = ?", team.ID, claims.UserID).Limit(1).Count(&n)
	if n > 0 {
		return "member"
	}
	return "none"
}

// listTeams: all logged-in users browse teams (same公开 policy as 题单).
func (s *Server) listTeams(c *gin.Context) {
	var teams []model.Team
	s.DB.Order("id DESC").Limit(200).Find(&teams)
	claims := auth.CurrentUser(c)
	out := make([]teamView, 0, len(teams))
	for _, t := range teams {
		var n int64
		s.DB.Model(&model.TeamMember{}).Where("team_id = ?", t.ID).Count(&n)
		tv := teamView{Team: t, MemberCount: n, Role: "none"}
		if captain := (&model.User{}); s.DB.Select("username, nickname").First(captain, t.CaptainID).Error == nil {
			tv.Captain = captain.Nickname
			if tv.Captain == "" {
				tv.Captain = captain.Username
			}
		}
		if claims != nil {
			tv.Role = s.teamRole(&t, claims)
		}
		out = append(out, tv)
	}
	c.JSON(200, out)
}

type createTeamReq struct {
	Name     string `json:"name" binding:"required,max=100"`
	Bio      string `json:"bio" binding:"max=300"`
	Capacity int    `json:"capacity"`
}

// createTeam: any logged-in user; creator becomes captain. Capacity is
// clamped to 1..5 with 3 as the ICPC default.
func (s *Server) createTeam(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	cap := req.Capacity
	if cap <= 0 {
		cap = 3
	}
	if cap > 5 {
		cap = 5
	}
	team := &model.Team{
		Name: req.Name, Bio: req.Bio, Capacity: cap,
		CaptainID: claims.UserID, InviteCode: randomCode(8),
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(team).Error; err != nil {
			return err
		}
		return tx.Create(&model.TeamMember{TeamID: team.ID, UserID: claims.UserID}).Error
	})
	if err != nil && (isUniqueViolation(err)) {
		c.JSON(400, gin.H{"error": "小组名已存在"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	c.JSON(200, teamView{Team: *team, MemberCount: 1, Role: "captain"})
}

func (s *Server) getTeam(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid team id"})
		return
	}
	team := &model.Team{}
	if err := s.DB.First(team, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "team not found"})
		return
	}
	var n int64
	s.DB.Model(&model.TeamMember{}).Where("team_id = ?", team.ID).Count(&n)
	claims := auth.CurrentUser(c)
	role := s.teamRole(team, claims)

	var notices []model.TeamNotice
	s.DB.Where("team_id = ?", team.ID).Order("id DESC").Limit(50).Find(&notices)

	// shared 题单 with per-caller progress summary
	var shares []model.TeamList
	s.DB.Where("team_id = ?", team.ID).Order("id DESC").Find(&shares)
	lists := make([]gin.H, 0, len(shares))
	for _, sh := range shares {
		l := &model.ProblemList{}
		if err := s.DB.First(l, sh.ListID).Error; err != nil {
			continue
		}
		var items int64
		s.DB.Model(&model.ProblemListItem{}).Where("list_id = ?", l.ID).Count(&items)
		lists = append(lists, gin.H{"id": l.ID, "title": l.Title, "description": l.Description, "items": items})
	}

	c.JSON(200, gin.H{
		"team":    teamView{Team: *team, MemberCount: n, Role: role, Members: s.teamMembers(team.ID)},
		"notices": notices, "lists": lists,
	})
}

// joinTeam: by invite code. Capacity (including the captain) is the hard cap.
func (s *Server) joinTeam(c *gin.Context) {
	claims := auth.CurrentUser(c)
	team := &model.Team{}
	if err := s.DB.Where("invite_code = ?", c.Param("code")).First(team).Error; err != nil {
		c.JSON(404, gin.H{"error": "邀请码无效"})
		return
	}
	if role := s.teamRole(team, claims); role != "none" {
		c.JSON(200, gin.H{"ok": true, "team_id": team.ID, "already": true})
		return
	}
	var n int64
	s.DB.Model(&model.TeamMember{}).Where("team_id = ?", team.ID).Count(&n)
	if n >= int64(team.Capacity) {
		c.JSON(400, gin.H{"error": "队伍已满（上限 " + itoa(team.Capacity) + " 人）"})
		return
	}
	if err := s.DB.Create(&model.TeamMember{TeamID: team.ID, UserID: claims.UserID}).Error; err != nil {
		c.JSON(500, gin.H{"error": "join failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "team_id": team.ID})
}

// pullMember: captain adds a user by id (same capacity rule).
func (s *Server) pullMember(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	target := &model.User{}
	if err := s.DB.First(target, uid).Error; err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	if role := s.teamRole(team, &auth.Claims{UserID: uid}); role != "none" {
		c.JSON(400, gin.H{"error": "该用户已在队伍中"})
		return
	}
	var n int64
	s.DB.Model(&model.TeamMember{}).Where("team_id = ?", team.ID).Count(&n)
	if n >= int64(team.Capacity) {
		c.JSON(400, gin.H{"error": "队伍已满（上限 " + itoa(team.Capacity) + " 人）"})
		return
	}
	if err := s.DB.Create(&model.TeamMember{TeamID: team.ID, UserID: uid}).Error; err != nil {
		c.JSON(500, gin.H{"error": "add failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// kickMember: captain removes someone else; captain cannot kick themselves
// (transfer first).
func (s *Server) kickMember(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	claims := auth.CurrentUser(c)
	if uid == claims.UserID {
		c.JSON(400, gin.H{"error": "队长不能移除自己，请先转让队长"})
		return
	}
	if err := s.DB.Delete(&model.TeamMember{}, "team_id = ? AND user_id = ?", team.ID, uid).Error; err != nil {
		c.JSON(500, gin.H{"error": "remove failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// leaveTeam: a member quits; the captain must transfer captaincy first if
// they are the last member.
func (s *Server) leaveTeam(c *gin.Context) {
	team := &model.Team{}
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid team id"})
		return
	}
	if err := s.DB.First(team, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "team not found"})
		return
	}
	claims := auth.CurrentUser(c)
	if team.CaptainID == claims.UserID {
		var members []model.TeamMember
		s.DB.Where("team_id = ?", team.ID).Find(&members)
		if len(members) <= 1 {
			// last member: leaving deletes the team entirely
			s.DB.Transaction(func(tx *gorm.DB) error {
				tx.Delete(&model.TeamMember{}, "team_id = ?", team.ID)
				tx.Delete(&model.TeamNotice{}, "team_id = ?", team.ID)
				tx.Delete(&model.TeamList{}, "team_id = ?", team.ID)
				tx.Delete(team)
				return nil
			})
			c.JSON(200, gin.H{"ok": true, "deleted": true})
			return
		}
		c.JSON(400, gin.H{"error": "队长需先转让队长再退出"})
		return
	}
	if err := s.DB.Delete(&model.TeamMember{}, "team_id = ? AND user_id = ?", team.ID, claims.UserID).Error; err != nil {
		c.JSON(500, gin.H{"error": "leave failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// transferCaptain: current captain hands the role to an existing member.
func (s *Server) transferCaptain(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	uid, okU := paramID(c, "uid")
	if !okU {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	if role := s.teamRole(team, &auth.Claims{UserID: uid}); role == "none" {
		c.JSON(400, gin.H{"error": "目标用户不是队员"})
		return
	}
	if err := s.DB.Model(team).Update("captain_id", uid).Error; err != nil {
		c.JSON(500, gin.H{"error": "transfer failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// regenerateInvite: captain rotates the team's invite code.
func (s *Server) regenerateInvite(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	code := randomCode(8)
	if err := s.DB.Model(team).Update("invite_code", code).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, gin.H{"invite_code": code})
}

// teamGuard: load team by :id and require the caller to be its captain.
func (s *Server) teamGuard(c *gin.Context) (*model.Team, bool) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid team id"})
		return nil, false
	}
	team := &model.Team{}
	if err := s.DB.First(team, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "team not found"})
		return nil, false
	}
	claims := auth.CurrentUser(c)
	if claims == nil || team.CaptainID != claims.UserID {
		c.JSON(403, gin.H{"error": "仅队长可执行此操作"})
		return nil, false
	}
	return team, true
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// createTeamNotice / deleteTeamNotice: captain-only announcements.
func (s *Server) createTeamNotice(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	claims := auth.CurrentUser(c)
	var req struct {
		Content string `json:"content" binding:"required,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	notice := &model.TeamNotice{TeamID: team.ID, UserID: claims.UserID, Content: req.Content}
	if err := s.DB.Create(notice).Error; err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	c.JSON(200, notice)
}

func (s *Server) deleteTeamNotice(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	nid, okN := paramID(c, "nid")
	if !okN {
		c.JSON(400, gin.H{"error": "invalid notice id"})
		return
	}
	if err := s.DB.Delete(&model.TeamNotice{}, "id = ? AND team_id = ?", nid, team.ID).Error; err != nil {
		c.JSON(500, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// teamBoard: 队内排行 — per-member distinct AC/tried counts, scoped to the
// problems covered by the team's shared 题单 (all problems when none shared).
// Sorted AC desc, tried desc, id asc.
func (s *Server) teamBoard(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid team id"})
		return
	}
	team := &model.Team{}
	if err := s.DB.First(team, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "team not found"})
		return
	}
	var members []model.TeamMember
	s.DB.Where("team_id = ?", team.ID).Order("id").Find(&members)

	var pids []uint
	var shares []model.TeamList
	s.DB.Where("team_id = ?", team.ID).Find(&shares)
	if len(shares) > 0 {
		listIDs := make([]uint, 0, len(shares))
		for _, sh := range shares {
			listIDs = append(listIDs, sh.ListID)
		}
		s.DB.Model(&model.ProblemListItem{}).Where("list_id IN ?", listIDs).Distinct().Pluck("problem_id", &pids)
	}

	type boardRow struct {
		UserID   uint   `json:"user_id"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		AC       int64  `json:"ac"`
		Tried    int64  `json:"tried"`
	}
	var users []model.User
	s.DB.Select("id, username, nickname").Find(&users)
	name := map[uint]model.User{}
	for _, u := range users {
		name[u.ID] = u
	}
	out := make([]boardRow, 0, len(members))
	for _, m := range members {
		row := boardRow{UserID: m.UserID}
		if u, ok := name[m.UserID]; ok {
			row.Username, row.Nickname = u.Username, u.Nickname
		}
		acQ := s.DB.Model(&model.Submission{}).
			Where("user_id = ? AND status = ? AND cancelled = ?", m.UserID, model.SubAC, false)
		triedQ := s.DB.Model(&model.Submission{}).
			Where("user_id = ? AND cancelled = ?", m.UserID, false)
		if pids != nil {
			acQ = acQ.Where("problem_id IN ?", pids)
			triedQ = triedQ.Where("problem_id IN ?", pids)
		}
		acQ.Distinct().Count(&row.AC)
		triedQ.Distinct().Count(&row.Tried)
		out = append(out, row)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			a, b := out[i], out[j]
			if a.AC < b.AC || (a.AC == b.AC && (a.Tried < b.Tried || (a.Tried == b.Tried && a.UserID > b.UserID))) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	c.JSON(200, out)
}

// shareList / unshareList: captain attaches/detaches a 题单 to the team.
func (s *Server) shareList(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	var req struct {
		ListID uint `json:"list_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	l := &model.ProblemList{}
	if err := s.DB.First(l, req.ListID).Error; err != nil {
		c.JSON(404, gin.H{"error": "list not found"})
		return
	}
	share := &model.TeamList{TeamID: team.ID, ListID: req.ListID}
	if err := s.DB.Where(share).FirstOrCreate(share).Error; err != nil {
		c.JSON(500, gin.H{"error": "share failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) unshareList(c *gin.Context) {
	team, ok := s.teamGuard(c)
	if !ok {
		return
	}
	lid, okL := paramID(c, "lid")
	if !okL {
		c.JSON(400, gin.H{"error": "invalid list id"})
		return
	}
	if err := s.DB.Delete(&model.TeamList{}, "team_id = ? AND list_id = ?", team.ID, lid).Error; err != nil {
		c.JSON(500, gin.H{"error": "unshare failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
