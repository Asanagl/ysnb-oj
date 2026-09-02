// Package handler — problem export (zip: statement + testdata) and
// problem copy for reusing authored problems across accounts/contests.
package handler

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// exportProblem streams a zip containing statement.md + testdata/.
// ?shared=1 switches to the fairness-safe export: statement + public
// samples only — the full judging testdata set NEVER leaves the system
// through this path (题目共享的红线：完整测试数据不出站).
func (s *Server) exportProblem(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canSeeProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	shared := c.Query("shared") == "1"
	c.Header("Content-Type", "application/zip")
	suffix := ""
	if shared {
		suffix = "-shared"
	}
	c.Header("Content-Disposition",
		`attachment; filename="problem-`+fmt.Sprint(prob.ID)+suffix+`.zip"`)
	zw := zip.NewWriter(c.Writer)

	statement, _ := zw.Create("statement.md")
	_, _ = statement.Write([]byte(prob.StatementMD))
	meta, _ := zw.Create("meta.txt")
	_, _ = fmt.Fprintf(meta, "id=%d\ntitle=%s\ntime_limit_ms=%d\nmem_limit_mb=%d\njudge_mode=%s\n",
		prob.ID, prob.Title, prob.TimeLimitMS, prob.MemLimitMB, prob.JudgeMode)
	// why include these: the importer restores judge_mode from the presence
	// of checker.cpp/interactor.cpp, so an export round-trip keeps SPJ/交互
	// configuration instead of silently downgrading to standard diff.
	// Shared exports omit them too: a checker encodes judging intent.
	if !shared {
		if prob.CheckerSource != "" {
			chk, _ := zw.Create("checker.cpp")
			_, _ = chk.Write([]byte(prob.CheckerSource))
		}
		if prob.InteractorSrc != "" {
			itc, _ := zw.Create("interactor.cpp")
			_, _ = itc.Write([]byte(prob.InteractorSrc))
		}
	}

	if shared {
		// fairness-safe variant: samples from the statement only
		samples := []model.Sample{}
		_ = json.Unmarshal([]byte(prob.Samples), &samples)
		for i, sm := range samples {
			in, _ := zw.Create(fmt.Sprintf("testdata/%d.in", i+1))
			_, _ = in.Write([]byte(sm.Input))
			out, _ := zw.Create(fmt.Sprintf("testdata/%d.out", i+1))
			_, _ = out.Write([]byte(sm.Output))
		}
	} else {
		dir := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID))
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				f, err := zw.Create("testdata/" + e.Name())
				if err == nil {
					if raw, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
						_, _ = f.Write(raw)
					}
				}
			}
		}
	}
	_ = zw.Close()
}

// copyProblem clones a bank problem (statement + limits + testdata) into a
// fresh problem owned by the caller — reuse authored work across contests.
func (s *Server) copyProblem(c *gin.Context) {
	claims := auth.CurrentUser(c)
	src, ok := s.problemByID(c)
	if !ok || !s.canManageProblem(c, src) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	clone := *src
	clone.ID = 0
	clone.Title = src.Title + "（副本）"
	clone.CreatedBy = claims.UserID
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = time.Now()
	clone.ContestID = nil
	if err := s.DB.Create(&clone).Error; err != nil {
		c.JSON(500, gin.H{"error": "copy failed"})
		return
	}
	var cases []model.TestCase
	if err := s.DB.Where("problem_id = ?", src.ID).Find(&cases).Error; err == nil {
		for _, tc := range cases {
			row := tc
			row.ID = 0
			row.ProblemID = clone.ID
			s.DB.Create(&row)
		}
	}
	copyDir(
		filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(src.ID)),
		filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(clone.ID)),
	)
	c.JSON(200, &clone)
}
