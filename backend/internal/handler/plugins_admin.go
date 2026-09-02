// Package handler — plugin system admin surface: plugin inventory, IOI
// per-case score table, and webhook event-hook target management.
package handler

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/plugin"
)

// pluginList serves GET /admin/plugins — everything registered, so the
// console can render the extension surface without guessing.
func (s *Server) pluginList(c *gin.Context) {
	hook := webhookTargetHook()
	c.JSON(200, gin.H{
		"problem_sources": plugin.ProblemSourceNames(),
		"submit_fetchers": plugin.SubmitFetcherNames(),
		"event_hooks":     []string{"webhook"},
		"hook_targets":    hook.Targets(),
	})
}

// webhookTargetHook resolves the built-in webhook hook from the registry.
func webhookTargetHook() *plugin.WebhookHook {
	h, ok := plugin.EventHookByID("webhook")
	if !ok {
		return nil
	}
	if wh, ok := h.(*plugin.WebhookHook); ok {
		return wh
	}
	return nil
}

// listHookTargets serves GET /admin/hooks.
func (s *Server) listHookTargets(c *gin.Context) {
	hook := webhookTargetHook()
	if hook == nil {
		c.JSON(200, gin.H{"targets": map[string]string{}})
		return
	}
	c.JSON(200, gin.H{"targets": hook.Targets()})
}

// setHookTarget serves PUT /admin/hooks/:name {"url": "..."}.
func (s *Server) setHookTarget(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required,max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	// SSRF guard: hook targets are outbound URLs — validate before storing.
	if err := plugin.ValidateHookURL(req.URL); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	hook := webhookTargetHook()
	if hook == nil {
		c.JSON(500, gin.H{"error": "webhook hook not registered"})
		return
	}
	hook.SetTarget(c.Param("name"), req.URL)
	c.JSON(200, gin.H{"targets": hook.Targets()})
}

// deleteHookTarget serves DELETE /admin/hooks/:name.
func (s *Server) deleteHookTarget(c *gin.Context) {
	hook := webhookTargetHook()
	if hook == nil {
		c.JSON(500, gin.H{"error": "webhook hook not registered"})
		return
	}
	hook.SetTarget(c.Param("name"), "")
	c.JSON(200, gin.H{"targets": hook.Targets()})
}

// --- IOI per-case score table ---

// setCaseScores serves PUT /problems/:id/case-scores — the IOI 分值表:
// {"scores": {"1": 20, "2": 30, ...}} or {"default": 100/N} semantics are
// expressed by the client sending explicit per-index values. Rows are
// upserted; scores apply only to IOI contests (ACM ignores them).
func (s *Server) setCaseScores(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canManageProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	var req struct {
		// Scores maps case index (as string for JSON) to its partial credit.
		Scores map[string]int `json:"scores" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Scores) == 0 {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	total := 0
	parsed := map[int]int{}
	for k, v := range req.Scores {
		idx, err := strconv.Atoi(k)
		if err != nil || idx < 1 || v < 0 {
			c.JSON(400, gin.H{"error": "invalid score entry: " + k})
			return
		}
		parsed[idx] = v
		total += v
	}
	if total == 0 {
		c.JSON(400, gin.H{"error": "total score must be positive"})
		return
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for idx, v := range parsed {
			res := tx.Model(&model.TestCase{}).
				Where("problem_id = ? AND case_index = ?", prob.ID, idx).
				Update("score", v)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("测试点 %d 不存在", idx)
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "total": total})
}

// listCaseScores serves GET /problems/:id/case-scores (setter+): the current
// per-case score table.
func (s *Server) listCaseScores(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canManageProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	var cases []model.TestCase
	s.DB.Where("problem_id = ?", prob.ID).Order("case_index").Find(&cases)
	out := map[string]int{}
	total := 0
	for _, tc := range cases {
		out[strconv.Itoa(tc.CaseIndex)] = tc.Score
		total += tc.Score
	}
	c.JSON(200, gin.H{"scores": out, "total": total, "cases": len(cases)})
}