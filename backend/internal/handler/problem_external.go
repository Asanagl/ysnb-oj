// Package handler — external problem import via plugin.ProblemSource
// (题库爬取). An admin/setter names a platform plugin + external id; the
// adapter fetches the public metadata (statement + samples) and a bank
// problem is created marked as external training material.
//
// Fairness red line: imported problems carry no judging testdata (external
// sites only publish samples) and default to members visibility with a
// source tag — they cannot masquerade as authored contest problems.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/plugin"
)

type importExternalReq struct {
	// Source is the plugin id, e.g. "codeforces" / "luogu".
	Source string `json:"source" binding:"required"`
	// ExternalID is the platform-native key, e.g. "1900A" / "P1001".
	ExternalID string `json:"external_id" binding:"required,max=64"`
	// Visibility defaults to members; public requires explicit opt-in.
	Visibility string `json:"visibility"`
}

// pluginSources lists registered crawlers (admin UI dropdown).
func (s *Server) pluginSources(c *gin.Context) {
	c.JSON(200, gin.H{
		"problem_sources": plugin.ProblemSourceNames(),
		"submit_fetchers": plugin.SubmitFetcherNames(),
	})
}

// externalProblemSources lists importable problem platforms for the all-user
// import UI — deliberately narrow: only the crawler names, no plugin
// internals, and it sits behind RequireAuth like the rest of the import flow.
func (s *Server) externalProblemSources(c *gin.Context) {
	c.JSON(200, gin.H{"problem_sources": plugin.ProblemSourceNames()})
}

// previewExternalProblem fetches metadata without creating anything — the
// setter sees what would be imported first.
func (s *Server) previewExternalProblem(c *gin.Context) {
	var req importExternalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	src, ok := plugin.ProblemSourceByID(req.Source)
	if !ok {
		c.JSON(400, gin.H{"error": "未知平台插件: " + req.Source})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	meta, err := src.FetchProblem(ctx, strings.TrimSpace(req.ExternalID))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"meta": meta})
}

// importExternalProblem creates the bank problem from a fetched ProblemMeta.
// No testdata rows are created — the problem is imported with its public
// samples only, so it is marked "外部题面/需自补数据" in Source and notes.
func (s *Server) importExternalProblem(c *gin.Context) {
	var req importExternalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	src, ok := plugin.ProblemSourceByID(req.Source)
	if !ok {
		c.JSON(400, gin.H{"error": "未知平台插件: " + req.Source})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	meta, err := src.FetchProblem(ctx, strings.TrimSpace(req.ExternalID))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	claims := auth.CurrentUser(c)
	// setter 及以上（setter/admin/super_admin）：免审核直接入正式题库
	canManage := claims.Role == model.RoleSetter || claims.Role == model.RoleAdmin ||
		claims.Role == model.RoleSuperAdmin
	visibility := orDefault(req.Visibility, model.VisibilityMembers)
	if visibility != model.VisibilityMembers && visibility != model.VisibilityHidden &&
		visibility != model.VisibilityPublic {
		c.JSON(400, gin.H{"error": "invalid visibility"})
		return
	}

	tagsJSON, _ := json.Marshal(meta.Tags)
	samplesJSON, _ := json.Marshal(samplesFromMeta(meta))
	sourceTag := fmt.Sprintf("[外部训练题·%s %s]", meta.Source, meta.ExternalID)
	prob := &model.Problem{
		Title:       orDefault(meta.Title, sourceTag),
		StatementMD: externalStatement(meta, sourceTag),
		InputDesc:   meta.InputDesc, OutputDesc: meta.OutputDesc, Hint: meta.Hint,
		Source:     fmt.Sprintf("%s %s", meta.Source, meta.ExternalID),
		Tags:       string(tagsJSON),
		Samples:    string(samplesJSON),
		TimeLimitMS: orDefaultInt(meta.TimeLimitMS, 1000),
		MemLimitMB:  orDefaultInt(meta.MemLimitMB, 256),
		Visibility:  visibility,
		JudgeMode:   model.JudgeModeDefault,
		CreatedBy:    claims.UserID,
	}
	// 审核流：一般用户导入的题与用户自建题一样先进待审核（隐藏），
	// 管理员审核通过后公开；setter 及以上免审核直接入正式题库。
	if canManage {
		prob.ReviewStatus = ReviewApproved
	} else {
		prob.ReviewStatus = ReviewPending
		prob.Visibility = model.VisibilityHidden
	}
	// Imported external problems carry no testdata; the SPA marks them
	// 「待补测试数据」(derived: external source + zero cases) until a
	// setter/admin uploads testdata.
	if strings.TrimSpace(prob.Title) == "" {
		prob.Title = sourceTag
	}
	if err := s.DB.Create(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "create problem failed"})
		return
	}
	c.JSON(200, gin.H{
		"problem": prob, "source": meta.Source, "external_id": meta.ExternalID,
		"notes":        meta.Notes,
		"needs_review": !canManage,
	})
}

// externalStatement assembles the imported markdown: original statement
// first, then a provenance banner so every viewer knows where it came from
// and why submitting may fail (no testdata yet).
func externalStatement(meta *plugin.ProblemMeta, sourceTag string) string {
	var b strings.Builder
	if strings.TrimSpace(meta.StatementMD) == "" {
		b.WriteString(sourceTag + "\n\n（题面抓取结果为空，请打开原题链接人工补充。）\n\n")
	} else {
		b.WriteString(sourceTag + "\n\n---\n\n")
		b.WriteString(meta.StatementMD)
		b.WriteString("\n\n---\n\n")
	}
	if meta.URL != "" {
		b.WriteString("**原题链接**: " + meta.URL + "\n\n")
	}
	b.WriteString("**注意**: 本题为外部平台导入的训练题，仅含公开样例；正式评测前需由出题人补充完整测试数据。\n")
	for _, n := range meta.Notes {
		b.WriteString("- " + n + "\n")
	}
	return b.String()
}

func samplesFromMeta(meta *plugin.ProblemMeta) []model.Sample {
	out := make([]model.Sample, 0, len(meta.Samples))
	for _, s := range meta.Samples {
		out = append(out, model.Sample{Input: s.Input, Output: s.Output, Note: s.Note})
	}
	return out
}

func orDefaultInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}