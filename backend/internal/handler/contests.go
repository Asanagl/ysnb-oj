package handler

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

const acmPenaltyPerWrong = 20 * time.Minute

type contestPayload struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	// Mode picks the scoring rules: "acm" (default) or "ioi" (partial
	// credit). Immutable after creation — mixing scoreboards mid-contest
	// would corrupt the standings math.
	Mode        string     `json:"mode"`
	Visibility  string     `json:"visibility"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     time.Time  `json:"end_time"`
	FreezeTime  *time.Time `json:"freeze_time"`
	// 封榜开关: false disables freeze entirely (freeze_time + manual lever inert).
	FreezeEnabled *bool `json:"freeze_enabled"`
	// 赛时手动封案权限: false forbids the jury's mid-contest freeze toggle.
	NoManualFreeze *bool `json:"no_manual_freeze"`
	// 报名开关: true requires 报名 before in-contest submissions (default true
	// for newly created contests).
	RequireRegistration *bool `json:"require_registration"`
	// 组队赛模式: true turns on team registration — contestants register as a
	// whole team, standings aggregate per team (ICPC 三人一队).
	TeamMode *bool `json:"team_mode"`
}

func (p *contestPayload) teamModeOption(contest *model.Contest, created bool) {
	if p.TeamMode != nil {
		contest.TeamMode = *p.TeamMode
		if !contest.TeamMode {
			contest.TeamCapacity = 0
		} else if contest.TeamCapacity == 0 {
			contest.TeamCapacity = 3
		}
	} else if created {
		contest.TeamMode = false
	}
}

func (p *contestPayload) freezeOptions(contest *model.Contest) {
	if p.FreezeEnabled != nil {
		contest.FreezeEnabled = *p.FreezeEnabled
		if !contest.FreezeEnabled {
			contest.FreezeTime = nil
			contest.ManualFrozen = false
			contest.ManualFrozenAt = nil
		}
	} else if contest.FreezeTime == nil && contest.ID == 0 {
		contest.FreezeEnabled = true // creation default
	}
	if p.NoManualFreeze != nil {
		contest.NoManualFreeze = *p.NoManualFreeze
		if contest.NoManualFreeze {
			contest.ManualFrozen = false
			contest.ManualFrozenAt = nil
		}
	} else if contest.ID == 0 {
		contest.NoManualFreeze = false
	}
}

func (p *contestPayload) registrationOption(contest *model.Contest, created bool) {
	if p.RequireRegistration != nil {
		contest.RequireRegistration = p.RequireRegistration
	} else if created {
		// new contests default to 报名制 (confirmed product decision)
		yes := true
		contest.RequireRegistration = &yes
	}
	// updates leave nil untouched: legacy contests keep their current mode
}

func (s *Server) createContest(c *gin.Context) {
	var payload contestPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Title == "" {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if !payload.EndTime.After(payload.StartTime) {
		c.JSON(400, gin.H{"error": "end_time must be after start_time"})
		return
	}
	claims := auth.CurrentUser(c)
	mode := model.ContestModeACM
	if payload.Mode == model.ContestModeIOI {
		mode = model.ContestModeIOI
	}
	contest := &model.Contest{
		Title: payload.Title, Description: payload.Description,
		Mode: mode, Visibility: orDefault(payload.Visibility, "public"),
		StartTime: payload.StartTime, EndTime: payload.EndTime,
		FreezeTime: payload.FreezeTime, CreatedBy: claims.UserID,
	}
	payload.freezeOptions(contest)
	payload.registrationOption(contest, true)
	payload.teamModeOption(contest, true)
	if err := s.DB.Create(contest).Error; err != nil {
		c.JSON(500, gin.H{"error": "create contest failed"})
		return
	}
	c.JSON(200, contest)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (s *Server) updateContest(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	if !canManageContest(c, contest) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var payload contestPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Title == "" {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	contest.Title = payload.Title
	contest.Description = payload.Description
	contest.Visibility = orDefault(payload.Visibility, "public")
	contest.StartTime = payload.StartTime
	contest.EndTime = payload.EndTime
	contest.FreezeTime = payload.FreezeTime
	payload.freezeOptions(contest)
	payload.registrationOption(contest, false)
	payload.teamModeOption(contest, false)
	s.DB.Save(contest)
	c.JSON(200, contest)
}

func canManageContest(c *gin.Context, contest *model.Contest) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	return isSetterRole(claims.Role) || contest.CreatedBy == claims.UserID
}

// canViewContest gates non-public contests to their managers, shared by the
// detail and standings endpoints so neither leaks a hidden contest.
func canViewContest(c *gin.Context, contest *model.Contest) bool {
	if contest.Visibility == "public" {
		return true
	}
	return canManageContest(c, contest)
}

func (s *Server) listContests(c *gin.Context) {
	claims := auth.CurrentUser(c)
	q := s.DB.Model(&model.Contest{})
	if claims == nil || !isSetterRole(claims.Role) {
		q = q.Where("visibility = ?", "public")
	}
	var contests []model.Contest
	q.Order("start_time DESC").Limit(100).Find(&contests)
	c.JSON(200, contests)
}

func (s *Server) getContest(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok || !canViewContest(c, contest) {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	var links []model.ContestProblem
	s.DB.Where("contest_id = ?", contest.ID).Order("order_index").Find(&links)
	problems := make([]gin.H, 0, len(links))
	for _, link := range links {
		prob := &model.Problem{}
		if err := s.DB.First(prob, link.ProblemID).Error; err != nil {
			continue
		}
		problems = append(problems, gin.H{
			"label": link.Label, "id": prob.ID, "title": prob.Title,
			"time_limit_ms": prob.TimeLimitMS, "mem_limit_mb": prob.MemLimitMB,
		})
	}
	var notices []model.ContestNotice
	s.DB.Where("contest_id = ?", contest.ID).Order("id DESC").Limit(50).Find(&notices)
	c.JSON(200, gin.H{
		"contest": contest, "problems": problems,
		"notices": notices, "is_judge": canJudgeContest(c, contest),
	})
}

// setContestProblems replaces the contest's problem set; labels A, B, C… are
// assigned by list order.
func (s *Server) setContestProblems(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	if !canManageContest(c, contest) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var req struct {
		ProblemIDs []uint `json:"problem_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	// Append-only attach: each id becomes an independent copy labelled after
	// the current set. why no destructive rebuild: exclusive problems created
	// inside this contest must survive re-saving the form.
	var current []model.ContestProblem
	if err := s.DB.Where("contest_id = ?", contest.ID).Order("order_index").Find(&current).Error; err != nil {
		c.JSON(500, gin.H{"error": "load contest problems failed"})
		return
	}
	type attachedProblem struct {
		label     string
		problemID uint
		pos       int
	}
	attachedList := make([]attachedProblem, 0, len(req.ProblemIDs))
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for i, pid := range req.ProblemIDs {
			src := &model.Problem{}
			if err := tx.First(src, pid).Error; err != nil {
				return fmt.Errorf("problem %d not found", pid)
			}
			var problemID uint
			switch {
			case src.ContestID != nil && *src.ContestID == contest.ID:
				// already an exclusive problem of THIS contest: link as-is
				problemID = src.ID
			case src.ContestID != nil:
				return fmt.Errorf("problem %d belongs to another contest", pid)
			default:
				clone, err := s.cloneProblemForContest(tx, pid, contest.ID)
				if err != nil {
					return err
				}
				problemID = clone.ID
			}
			pos := len(current) + i
			attachedList = append(attachedList, attachedProblem{
				label: labelFor(pos), problemID: problemID, pos: pos,
			})
		}
		for _, a := range attachedList {
			link := &model.ContestProblem{
				ContestID: contest.ID, ProblemID: a.problemID,
				Label: a.label, OrderIndex: a.pos,
			}
			if err := tx.Create(link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "copied": len(attachedList)})
}

// cloneProblemForContest duplicates a bank problem into an exclusive contest
// copy: a new Problem row plus its testdata files and test-case metadata.
func (s *Server) cloneProblemForContest(tx *gorm.DB, srcID, contestID uint) (*model.Problem, error) {
	src := &model.Problem{}
	if err := tx.First(src, srcID).Error; err != nil {
		return nil, fmt.Errorf("source problem %d not found", srcID)
	}
	if src.ContestID != nil {
		return nil, fmt.Errorf("problem %d already belongs to a contest", srcID)
	}
	cid := contestID
	clone := *src
	clone.ID = 0
	clone.ContestID = &cid
	clone.Visibility = model.VisibilityHidden
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = time.Now()
	if err := tx.Create(&clone).Error; err != nil {
		return nil, err
	}
	// clone test-case metadata, then the on-disk blobs
	var cases []model.TestCase
	if err := tx.Where("problem_id = ?", srcID).Order("case_index").Find(&cases).Error; err != nil {
		return nil, err
	}
	for _, tc := range cases {
		row := tc
		row.ID = 0
		row.ProblemID = clone.ID
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
	}
	if err := copyDir(
		filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(srcID)),
		filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(clone.ID)),
	); err != nil {
		return nil, err
	}
	return &clone, nil
}

func copyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // problem without testdata clones empty
		}
		return err
	}
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), raw, 0o640); err != nil {
			return err
		}
	}
	return nil
}

func labelFor(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return fmt.Sprintf("A%d", i-25)
}

// exportStandingsCSV streams the current board as CSV for 公示/存档.
func (s *Server) exportStandingsCSV(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok || !canViewContest(c, contest) {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	var links []model.ContestProblem
	s.DB.Where("contest_id = ?", contest.ID).Order("order_index").Find(&links)
	var subs []model.Submission
	s.DB.Where("contest_id = ? AND is_practice = ?", contest.ID, false).Find(&subs)
	var flags []model.ContestUserFlag
	s.DB.Where("contest_id = ?", contest.ID).Find(&flags)
	flagMap := map[uint]model.ContestUserFlag{}
	for _, f := range flags {
		flagMap[f.UserID] = f
	}
	rows := computeStandings(contest, links, subs, flagMap, time.Now(), contest.RevealCount)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition",
		`attachment; filename="standings-contest-`+fmt.Sprint(contest.ID)+`.csv"`)
	w := csv.NewWriter(c.Writer)
	header := []string{"rank", "user_id", "solved", "penalty_min", "flag"}
	for _, l := range links {
		header = append(header, l.Label)
	}
	_ = w.Write(header)
	for _, r := range rows {
		flagWord := ""
		if r.Cheated {
			flagWord = "cheated"
		} else if r.Starred {
			flagWord = "star"
		}
		rankWord := strconv.Itoa(r.Rank)
		if r.Cheated {
			rankWord = "cheated"
		} else if r.Starred {
			rankWord = "*"
		}
		record := []string{rankWord, fmt.Sprint(r.UserID),
			fmt.Sprint(r.Solved), fmt.Sprint(r.PenaltyMS / 60000), flagWord}
		for _, l := range links {
			cell := r.Cells[l.Label]
			record = append(record, cellTextCSV(cell))
		}
		_ = w.Write(record)
	}
	w.Flush()
}

func cellTextCSV(cell *standingCell) string {
	if cell == nil {
		return ""
	}
	if cell.Solved {
		return fmt.Sprintf("+%d", cell.SolvedMS/60000)
	}
	if cell.Pending > 0 {
		return fmt.Sprintf("?%d", cell.Pending)
	}
	if cell.Attempts > 0 {
		return fmt.Sprintf("-%d", cell.Attempts)
	}
	return ""
}

// getStandings computes ACM-rank rows; frozen submissions appear only as
// pending counts until the contest ends. Visibility matches getContest —
// a hidden contest must not leak its standings either (BUG-004).
func (s *Server) getStandings(c *gin.Context) {
	contest, ok := s.contestByID(c)
	if !ok || !canViewContest(c, contest) {
		c.JSON(404, gin.H{"error": "contest not found"})
		return
	}
	var links []model.ContestProblem
	s.DB.Where("contest_id = ?", contest.ID).Order("order_index").Find(&links)
	// 补题 submissions never touch the frozen standings.
	var subs []model.Submission
	s.DB.Where("contest_id = ? AND is_practice = ?", contest.ID, false).Find(&subs)
	var flags []model.ContestUserFlag
	s.DB.Where("contest_id = ?", contest.ID).Find(&flags)
	flagMap := map[uint]model.ContestUserFlag{}
	for _, f := range flags {
		flagMap[f.UserID] = f
	}
	rows := computeStandings(contest, links, subs, flagMap, time.Now(), contest.RevealCount)
	// fill display names so the board shows nicknames instead of bare ids
	var users []model.User
	s.DB.Select("id, username, nickname").Find(&users)
	nameOf := map[uint]string{}
	for _, u := range users {
		if u.Nickname != "" {
			nameOf[u.ID] = u.Nickname
		} else {
			nameOf[u.ID] = u.Username
		}
	}
	for i := range rows {
		rows[i].Username = nameOf[rows[i].UserID]
	}
	// 组队赛: aggregate member rows into one standing row per registered
	// team. Why post-processing instead of rewriting computeStandings: the
	// pure function stays user-keyed and unit-tested; team merge is a thin
	// presentation-layer fold over the same cells.
	if contest.TeamMode {
		rows = s.teamStandings(contest, rows)
	}
	c.JSON(200, gin.H{
		"rows":         rows,
		"reveal_count": contest.RevealCount,
		"frozen":       contestFrozen(contest, time.Now()),
		"team_mode":    contest.TeamMode,
	})
}

// teamStandings folds user rows into team rows. Cell merge rules (ACM):
// solved if ANY member solved it, at the earliest solve time + the smallest
// combined wrong-attempt penalty across members; attempts count the member
// minimums before the first team solve. Starred/cheated follow the captain's
// flag (jury marks a team via any member flag, convention: the captain's).
func (s *Server) teamStandings(contest *model.Contest, rows []standingRow) []standingRow {
	// map user -> team
	var regs []model.ContestRegistration
	s.DB.Where("contest_id = ? AND team_id > 0", contest.ID).Find(&regs)
	userTeam := map[uint]uint{}
	for _, r := range regs {
		userTeam[r.UserID] = r.TeamID
	}
	if len(userTeam) == 0 {
		return rows
	}
	teams := map[uint]*standingRow{}
	memberCount := map[uint]int{}
	byID := map[uint]*standingRow{}
	for i := range rows {
		byID[rows[i].UserID] = &rows[i]
	}
	var teamIDs []uint
	for _, r := range rows {
		tid, ok := userTeam[r.UserID]
		if !ok {
			continue // an individual straggler row — already blocked at registration
		}
		t := teams[tid]
		if t == nil {
			t = &standingRow{UserID: tid, Cells: map[string]*standingCell{}}
			teams[tid] = t
			teamIDs = append(teamIDs, tid)
		}
		memberCount[tid]++
		t.Solved += r.Solved
		t.PenaltyMS += r.PenaltyMS
		for label, cell := range r.Cells {
			cur := t.Cells[label]
			if cur == nil {
				cc := *cell
				t.Cells[label] = &cc
				continue
			}
			if cell.Solved && (!cur.Solved || cell.SolvedMS < cur.SolvedMS) {
				merged := *cell
				merged.Attempts = cur.Attempts
				t.Cells[label] = &merged
			} else if !cell.Solved && !cur.Solved {
				// combine wrong attempts; pending cannot exceed members
				merged := *cur
				merged.Attempts += cell.Attempts
				if cell.Pending+cur.Pending > 0 {
					merged.Pending = 1
				}
				t.Cells[label] = &merged
			}
		}
	}
	// recompute solved/penalty from merged cells for ACM consistency
	var teamNames = map[uint]string{}
	var teamTeams []model.Team
	s.DB.Find(&teamTeams)
	for _, t := range teamTeams {
		teamNames[t.ID] = t.Name
	}
	var teamFlags = map[uint]model.ContestUserFlag{}
	var flags []model.ContestUserFlag
	s.DB.Where("contest_id = ?", contest.ID).Find(&flags)
	var captains = map[uint]uint{}
	for _, t := range teamTeams {
		captains[t.ID] = t.CaptainID
	}
	for _, f := range flags {
		teamFlags[f.UserID] = f
	}
	out := make([]standingRow, 0, len(teams))
	for tid, t := range teams {
		t.Solved, t.PenaltyMS = 0, 0
		for _, cell := range t.Cells {
			if cell.Solved {
				t.Solved++
				t.PenaltyMS += cell.SolvedMS
			}
		}
		t.Username = teamNames[tid]
		cap := captains[tid]
		if f, ok := teamFlags[cap]; ok {
			t.Starred, t.Cheated = f.Starred, f.Cheated
		}
		out = append(out, *t)
	}
	// rank: official first, then ★, then cheated (same tier rule as users)
	tier := func(r standingRow) int {
		if r.Cheated {
			return 2
		}
		if r.Starred {
			return 1
		}
		return 0
	}
	sort.Slice(out, func(i, j int) bool {
		if tier(out[i]) != tier(out[j]) {
			return tier(out[i]) < tier(out[j])
		}
		if out[i].Solved != out[j].Solved {
			return out[i].Solved > out[j].Solved
		}
		if out[i].PenaltyMS != out[j].PenaltyMS {
			return out[i].PenaltyMS < out[j].PenaltyMS
		}
		return out[i].UserID < out[j].UserID
	})
	rank := 0
	for i := range out {
		if tier(out[i]) == 0 {
			rank++
			out[i].Rank = rank
		}
	}
	return out
}

type standingCell struct {
	Label    string `json:"label"`
	Attempts int    `json:"attempts"`
	SolvedMS int64  `json:"solved_ms"`
	Pending  int    `json:"pending"` // frozen, verdict hidden
	Solved   bool   `json:"solved"`
}

type standingRow struct {
	UserID    uint                     `json:"user_id"`
	Username  string                   `json:"username"`
	Solved    int                      `json:"solved"`
	PenaltyMS int64                    `json:"penalty_ms"`
	Cells     map[string]*standingCell `json:"cells"`
	Rank      int                      `json:"rank"` // 0 = unrated (★/cheated)
	Starred   bool                     `json:"starred"`
	Cheated   bool                     `json:"cheated"`
}

// computeStandings is pure so it can be unit-tested: pre-freeze ACs count;
// post-freeze activity on unsolved problems shows as pending; jury-cancelled
// submissions are invisible; cheated users keep an empty flagged row at the
// bottom; starred users rank normally but carry no rank number and sort
// after rated rows. revealCount (滚榜) un-masks the last-N ranked rows from
// the true final board — their post-freeze verdicts count again.
func computeStandings(contest *model.Contest, links []model.ContestProblem,
	subs []model.Submission, flags map[uint]model.ContestUserFlag,
	now time.Time, revealCount int) []standingRow {
	if contest.Mode == model.ContestModeIOI {
		return computeIOIStandings(contest, links, subs, flags, now, revealCount)
	}
	labels := map[uint]string{}
	for _, l := range links {
		labels[l.ProblemID] = l.Label
	}
	frozen := contestFrozen(contest, now)
	// 滚榜: derive the reveal set from the true final board — bottom-up,
	// official rows only (★/作弊 stay masked; they are not part of the roll).
	revealed := map[uint]bool{}
	if frozen && revealCount > 0 {
		unfrozen := *contest
		unfrozen.FreezeEnabled = true
		unfrozen.ManualFrozen = false
		unfrozen.RevealCount = 0
		trueRows := computeStandings(&unfrozen, links, subs, flags, now, 0)
		for i := len(trueRows) - 1; i >= 0 && len(revealed) < revealCount; i-- {
			if !trueRows[i].Cheated && !trueRows[i].Starred {
				revealed[trueRows[i].UserID] = true
			}
		}
	}
	fp := freezePoint(contest, now)
	rows := map[uint]*standingRow{}
	for i := range subs {
		sub := subs[i]
		if sub.Cancelled {
			continue // jury-nullified: invisible to the standings math
		}
		if flags[sub.UserID].Cheated {
			continue // flagged users keep an empty row, scored nothing
		}
		row := rows[sub.UserID]
		if row == nil {
			row = &standingRow{UserID: sub.UserID, Cells: map[string]*standingCell{}}
			rows[sub.UserID] = row
		}
		label, ok := labels[sub.ProblemID]
		if !ok {
			continue
		}
		cell := row.Cells[label]
		if cell == nil {
			cell = &standingCell{Label: label}
			row.Cells[label] = cell
		}
		if cell.Solved {
			continue // later submissions never change an ACM cell
		}
		// 滚榜-revealed users' submissions read as unfrozen (their verdicts
		// have been publicly revealed), everyone else stays masked.
		isFrozenSub := frozen && !revealed[sub.UserID] && fp != nil && sub.CreatedAt.After(*fp)
		if sub.Status == model.SubAC {
			solveMS := sub.CreatedAt.Sub(contest.StartTime).Milliseconds()
			applySolve(row, cell, solveMS, isFrozenSub)
			continue
		}
		if isFrozenSub {
			cell.Pending++
			continue
		}
		if isFailedStatus(sub.Status) {
			cell.Attempts++
		}
	}
	// Cheated users must stay visible with their flag even if we skipped all
	// of their submissions — the row is the public record of the verdict.
	for uid, f := range flags {
		if f.Cheated {
			if _, ok := rows[uid]; !ok {
				rows[uid] = &standingRow{UserID: uid, Cells: map[string]*standingCell{}}
			}
		}
	}
	return rankRows(rows, flags)
}

// contestFrozen: automatic schedule OR the jury's manual lever, never after
// the contest has ended.
// contestFrozen: automatic schedule OR the jury's manual lever — but only
// when the contest has 封榜 enabled at all.
func contestFrozen(contest *model.Contest, now time.Time) bool {
	if !contest.FreezeEnabled || now.After(contest.EndTime) {
		return false
	}
	if contest.ManualFrozen {
		return true
	}
	return contest.FreezeTime != nil && now.After(*contest.FreezeTime)
}

func isFailedStatus(status string) bool {
	switch status {
	case model.SubWA, model.SubTLE, model.SubMLE, model.SubRE:
		return true
	}
	return false
}

// applySolve records an AC; during the freeze it only bumps the pending
// counter so the verdict stays hidden until unfreeze.
func applySolve(row *standingRow, cell *standingCell, solveMS int64, frozenSolve bool) {
	if frozenSolve {
		cell.Pending++
		return
	}
	cell.Solved = true
	cell.SolvedMS = solveMS + int64(cell.Attempts)*acmPenaltyPerWrong.Milliseconds()
	row.Solved++
	row.PenaltyMS += cell.SolvedMS
}

// rankRows orders the board and assigns rank numbers: rated rows first
// (1..n), starred rows after them with no rank number, cheated rows last.
func rankRows(rows map[uint]*standingRow, flags map[uint]model.ContestUserFlag) []standingRow {
	out := make([]standingRow, 0, len(rows))
	for _, r := range rows {
		if f, ok := flags[r.UserID]; ok {
			r.Starred, r.Cheated = f.Starred, f.Cheated
		}
		out = append(out, *r)
	}
	tier := func(r standingRow) int {
		if r.Cheated {
			return 2
		}
		if r.Starred {
			return 1
		}
		return 0
	}
	sort.Slice(out, func(i, j int) bool {
		if tier(out[i]) != tier(out[j]) {
			return tier(out[i]) < tier(out[j])
		}
		if out[i].Solved != out[j].Solved {
			return out[i].Solved > out[j].Solved
		}
		if out[i].PenaltyMS != out[j].PenaltyMS {
			return out[i].PenaltyMS < out[j].PenaltyMS
		}
		return out[i].UserID < out[j].UserID
	})
	rank := 0
	for i := range out {
		if tier(out[i]) == 0 {
			rank++
			out[i].Rank = rank
		}
	}
	return out
}

// computeIOIStandings scores an IOI-mode board: each problem's cell carries
// the user's best partial score (max over that problem's submissions; the
// standingCell.SolvedMS field is reused as the score slot and the frontend
// renders it as 分数 in IOI contests). Rows rank by total score desc, ties
// broken by earlier last-score-improvement (stable tie: user id). The freeze
// still masks submissions made after the freeze point: their score reads as
// pending (cell.Pending=1, contribution 0) until unfreeze/reveal.
func computeIOIStandings(contest *model.Contest, links []model.ContestProblem,
	subs []model.Submission, flags map[uint]model.ContestUserFlag,
	now time.Time, revealCount int) []standingRow {
	labels := map[uint]string{}
	for _, l := range links {
		labels[l.ProblemID] = l.Label
	}
	frozen := contestFrozen(contest, now)
	// 滚榜 for IOI mirrors ACM: derive the reveal set from the true board.
	revealed := map[uint]bool{}
	if frozen && revealCount > 0 {
		unfrozen := *contest
		unfrozen.FreezeEnabled = true
		unfrozen.ManualFrozen = false
		unfrozen.RevealCount = 0
		trueRows := computeIOIStandings(&unfrozen, links, subs, flags, now, 0)
		for i := len(trueRows) - 1; i >= 0 && len(revealed) < revealCount; i-- {
			if !trueRows[i].Cheated && !trueRows[i].Starred {
				revealed[trueRows[i].UserID] = true
			}
		}
	}
	fp := freezePoint(contest, now)
	rows := map[uint]*standingRow{}
	for i := range subs {
		sub := subs[i]
		if sub.Cancelled {
			continue
		}
		if flags[sub.UserID].Cheated {
			continue
		}
		row := rows[sub.UserID]
		if row == nil {
			row = &standingRow{UserID: sub.UserID, Cells: map[string]*standingCell{}}
			rows[sub.UserID] = row
		}
		label, ok := labels[sub.ProblemID]
		if !ok {
			continue
		}
		cell := row.Cells[label]
		if cell == nil {
			cell = &standingCell{Label: label}
			row.Cells[label] = cell
		}
		isFrozenSub := frozen && !revealed[sub.UserID] && fp != nil && sub.CreatedAt.After(*fp)
		if isFrozenSub {
			// a frozen submission may have raised the score; mask it as one
			// pending marker but never lower the shown (pre-freeze) score.
			cell.Pending++
			continue
		}
		// best-of: max score across this problem's submissions (attempts
		// count every non-cancelled judged submission).
		cell.Attempts++
		if int64(sub.Score) > cell.SolvedMS {
			cell.SolvedMS = int64(sub.Score)
		}
		if sub.Status == model.SubAC {
			cell.Solved = true
		}
	}
	for _, row := range rows {
		total := int64(0)
		solvedCount := 0
		for _, cell := range row.Cells {
			total += cell.SolvedMS
			if cell.Solved {
				solvedCount++
			}
		}
		row.Solved = solvedCount
		row.PenaltyMS = total // reused as the IOI total score in rows
	}
	for uid, f := range flags {
		if f.Cheated {
			if _, ok := rows[uid]; !ok {
				rows[uid] = &standingRow{UserID: uid, Cells: map[string]*standingCell{}}
			}
		}
	}
	out := rankRows(rows, flags)
	// IOI rank order: total score desc (PenaltyMS holds the total). Re-sort
	// with the IOI comparator while preserving tier ordering from rankRows.
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := tierOf(out[i]), tierOf(out[j])
		if ti != tj {
			return ti < tj
		}
		if out[i].PenaltyMS != out[j].PenaltyMS {
			return out[i].PenaltyMS > out[j].PenaltyMS
		}
		return out[i].UserID < out[j].UserID
	})
	rank := 0
	for i := range out {
		if tierOf(out[i]) == 0 {
			rank++
			out[i].Rank = rank
		}
	}
	return out
}

func tierOf(r standingRow) int {
	if r.Cheated {
		return 2
	}
	if r.Starred {
		return 1
	}
	return 0
}
