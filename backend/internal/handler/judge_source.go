// Package handler — checker/interactor source file upload. V1 kept these as
// paste-only textareas; ICPC tool chains keep them as files, so accept .cpp
// uploads the same way testdata zips are accepted.
package handler

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

const maxJudgeSourceBytes = 512 << 10 // 512KB is generous for a checker

// uploadJudgeSource handles POST /problems/:id/(checker|interactor) with a
// "file" multipart field; the source is stored on the Problem row exactly as
// the paste path would (judge compiles from DB text).
func (s *Server) uploadJudgeSource(c *gin.Context, kind string) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	if kind == "checker" && prob.JudgeMode != model.JudgeModeSPJ {
		c.JSON(400, gin.H{"error": "题目判题模式不是 spj，请先切换判题模式"})
		return
	}
	if kind == "interactor" && prob.JudgeMode != model.JudgeModeInteractive {
		c.JSON(400, gin.H{"error": "题目判题模式不是 interactive，请先切换判题模式"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "missing file field"})
		return
	}
	if fileHeader.Size > maxJudgeSourceBytes {
		c.JSON(400, gin.H{"error": fmt.Sprintf("源码过大（上限 %d KB）", maxJudgeSourceBytes>>10)})
		return
	}
	name := strings.ToLower(fileHeader.Filename)
	if !strings.HasSuffix(name, ".cpp") && !strings.HasSuffix(name, ".cc") && !strings.HasSuffix(name, ".cxx") {
		c.JSON(400, gin.H{"error": "仅支持 C++ 源码文件（.cpp/.cc/.cxx）"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "open upload failed"})
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxJudgeSourceBytes+1))
	if err != nil || int64(len(raw)) > maxJudgeSourceBytes {
		c.JSON(400, gin.H{"error": "read file failed"})
		return
	}
	if kind == "checker" {
		prob.CheckerSource = string(raw)
	} else {
		prob.InteractorSrc = string(raw)
	}
	if err := s.DB.Save(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "save failed"})
		return
	}
	c.JSON(200, gin.H{
		"ok": true, "kind": kind,
		"filename": filepath.Base(fileHeader.Filename),
		"bytes":    len(raw),
	})
}
