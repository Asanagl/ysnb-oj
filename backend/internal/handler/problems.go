package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

type problemPayload struct {
	Title       string         `json:"title"`
	StatementMD string         `json:"statement_md"`
	InputDesc   string         `json:"input_desc"`
	OutputDesc  string         `json:"output_desc"`
	Hint        string         `json:"hint"`
	Source      string         `json:"source"`
	Tags        []string       `json:"tags"`
	Samples     []model.Sample `json:"samples"`
	TimeLimitMS int            `json:"time_limit_ms"`
	MemLimitMB  int            `json:"mem_limit_mb"`
	Visibility  string         `json:"visibility"`
	JudgeMode   string         `json:"judge_mode"`
	// SPJ checker / interactive interactor sources (C++17); the judge
	// compiles them per problem and caches by hash.
	CheckerSource    string `json:"checker_source"`
	InteractorSource string `json:"interactor_source"`
	// ContestID optionally creates the problem as an exclusive contest
	// problem (never listed in the public bank). Requires manage rights.
	ContestID *uint `json:"contest_id"`
}

func (p *problemPayload) normalize() error {
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if p.TimeLimitMS <= 0 {
		p.TimeLimitMS = 1000
	}
	if p.MemLimitMB <= 0 {
		p.MemLimitMB = 256
	}
	switch p.Visibility {
	case model.VisibilityHidden, model.VisibilityMembers, model.VisibilityPublic:
	default:
		p.Visibility = model.VisibilityMembers
	}
	switch p.JudgeMode {
	case model.JudgeModeDefault, model.JudgeModeSPJ, model.JudgeModeInteractive:
	default:
		p.JudgeMode = model.JudgeModeDefault
	}
	return nil
}

func (p *problemPayload) toModel(existing *model.Problem) *model.Problem {
	prob := existing
	if prob == nil {
		prob = &model.Problem{}
	}
	tags, _ := json.Marshal(p.Tags)
	samples, _ := json.Marshal(p.Samples)
	prob.Title = p.Title
	prob.StatementMD = p.StatementMD
	prob.InputDesc = p.InputDesc
	prob.OutputDesc = p.OutputDesc
	prob.Hint = p.Hint
	prob.Source = p.Source
	prob.Tags = string(tags)
	prob.Samples = string(samples)
	prob.TimeLimitMS = p.TimeLimitMS
	prob.MemLimitMB = p.MemLimitMB
	prob.Visibility = p.Visibility
	prob.JudgeMode = p.JudgeMode
	// why overwrite unconditionally: checker/interactor are judge-managed
	// fields, but the managing client sends the full problem each time (PUT
	// semantics), so partial updates would silently resurrect stale code.
	prob.CheckerSource = p.CheckerSource
	prob.InteractorSrc = p.InteractorSource
	return prob
}

func (s *Server) listProblems(c *gin.Context) {
	// why exclude contest problems here: exclusive contest problems live in
	// their contest only — the bank stays clean per the 题库/比赛分离 model.
	q := visibleProblemScope(s.DB, c).Where("contest_id IS NULL")
	if kw := c.Query("q"); kw != "" {
		q = q.Where("title LIKE ?", "%"+kw+"%")
	}
	if tag := c.Query("tag"); tag != "" {
		q = q.Where("tags LIKE ?", "%\""+tag+"\"%")
	}
	var total int64
	q.Count(&total)
	var problems []model.Problem
	page, size := pageParams(c)
	q.Order("id").Offset((page - 1) * size).Limit(size).Find(&problems)
	c.JSON(200, gin.H{"total": total, "items": problems})
}

func (s *Server) getProblem(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canSeeProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	scases := []model.Sample{}
	_ = json.Unmarshal([]byte(prob.Samples), &scases)
	var caseCount int64
	s.DB.Model(&model.TestCase{}).Where("problem_id = ?", prob.ID).Count(&caseCount)
	resp := gin.H{
		"problem": prob, "samples": scases, "case_count": caseCount,
		"can_manage": s.canManageProblem(c, prob),
	}
	if s.canManageProblem(c, prob) {
		resp["checker_source"] = prob.CheckerSource
		resp["interactor_source"] = prob.InteractorSrc
	}
	c.JSON(200, resp)
}

func (s *Server) canSeeProblem(c *gin.Context, prob *model.Problem) bool {
	// Contest-exclusive problems bypass the visibility field entirely: they
	// are viewable once their contest has started (补题 keeps them open).
	if prob.ContestID != nil {
		if s.canManageProblem(c, prob) {
			return true
		}
		contest := &model.Contest{}
		if err := s.DB.First(contest, *prob.ContestID).Error; err != nil {
			return false
		}
		return canViewContest(c, contest) && !time.Now().Before(contest.StartTime)
	}
	if prob.Visibility != model.VisibilityHidden {
		return true
	}
	// Review flow: the author may see (and edit) their own pending/rejected
	// problem, but must NOT be able to submit to it before approval — hence
	// this check grants visibility only, and createSubmission separately
	// blocks non-approved problems from judging (see below).
	if prob.CreatedBy == auth.CurrentUser(c).UserID &&
		(prob.ReviewStatus == ReviewPending || prob.ReviewStatus == ReviewRejected || prob.ReviewStatus == ReviewDraft) {
		return true
	}
	return s.canManageProblem(c, prob)
}

func (s *Server) canManageProblem(c *gin.Context, prob *model.Problem) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	if isSetterRole(claims.Role) {
		return true
	}
	if prob.CreatedBy == claims.UserID {
		return true
	}
	// why contest-manager path: exclusive problems are owned by their
	// contest, so its managers must be able to edit them.
	if prob.ContestID != nil {
		contest := &model.Contest{}
		if err := s.DB.First(contest, *prob.ContestID).Error; err == nil {
			return canManageContest(c, contest)
		}
	}
	return false
}

func (s *Server) createProblem(c *gin.Context) {
	var payload problemPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.normalize() != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	// Exclusive contest problem: the creator must manage the target contest;
	// such problems are born outside the public bank.
	if payload.ContestID != nil {
		contest := &model.Contest{}
		if err := s.DB.First(contest, *payload.ContestID).Error; err != nil {
			c.JSON(404, gin.H{"error": "contest not found"})
			return
		}
		if !canManageContest(c, contest) {
			c.JSON(403, gin.H{"error": "not allowed"})
			return
		}
		payload.Visibility = model.VisibilityHidden
	}
	claims := auth.CurrentUser(c)
	prob := payload.toModel(nil)
	prob.ContestID = payload.ContestID
	prob.CreatedBy = claims.UserID
	if err := s.DB.Create(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "create problem failed"})
		return
	}
	c.JSON(200, prob)
}

func (s *Server) updateProblem(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var payload problemPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.normalize() != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	payload.toModel(prob)
	if err := s.DB.Save(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, prob)
}

func (s *Server) deleteProblem(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	s.DB.Transaction(func(tx *gorm.DB) error {
		tx.Delete(&model.TestCase{}, "problem_id = ?", prob.ID)
		return tx.Delete(prob).Error
	})
	os.RemoveAll(filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID)))
	c.JSON(200, gin.H{"ok": true})
}

// uploadTestdata accepts a zip whose entries are <n>.in / <n>.out pairs
// (DOMjudge/Kattis style); entries are streamed to disk under testdata/.
func (s *Server) uploadTestdata(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "missing file field"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "open upload failed"})
		return
	}
	defer f.Close()
	result, err := s.storeTestdataZip(prob, f)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
}

func (s *Server) listTestdata(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var cases []model.TestCase
	s.DB.Where("problem_id = ?", prob.ID).Order("case_index").Find(&cases)
	c.JSON(200, cases)
}

// deleteTestdata removes one test case of THIS problem only (BUG-002: the
// row id alone must never cross problem boundaries), propagates DB errors,
// and cleans the on-disk blobs so daemons can't fetch stale data.
func (s *Server) deleteTestdata(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	caseID, ok := paramID(c, "caseId")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid case id"})
		return
	}
	tc := &model.TestCase{}
	if err := s.DB.First(tc, caseID).Error; err != nil {
		c.JSON(404, gin.H{"error": "test case not found"})
		return
	}
	if tc.ProblemID != prob.ID {
		c.JSON(404, gin.H{"error": "test case not found"})
		return
	}
	if err := s.DB.Delete(tc).Error; err != nil {
		c.JSON(500, gin.H{"error": "delete test case failed"})
		return
	}
	destDir := filepath.Join(s.Cfg.DataDir, "testdata", strconv.FormatUint(uint64(prob.ID), 10))
	_ = os.Remove(filepath.Join(destDir, strconv.Itoa(int(tc.CaseIndex))+".in"))
	_ = os.Remove(filepath.Join(destDir, strconv.Itoa(int(tc.CaseIndex))+".out"))
	c.JSON(200, gin.H{"ok": true})
}
