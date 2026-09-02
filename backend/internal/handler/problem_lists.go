// Package handler — training 题单 (problem lists): ordered lists of problems
// every logged-in user can browse (site-wide public per product decision).
// Progress (未做/尝试过/已AC) is derived from the caller's own submissions at
// read time — nothing to invalidate, practice submissions included.
package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

type listSummary struct {
	model.ProblemList
	ProblemCount int64 `json:"problem_count"`
}

type listItemView struct {
	Item model.ProblemListItem `json:"item"`
	ProblemBrief
	// caller's progress on this problem: todo | tried | ac
	Progress string `json:"progress"`
}

// ProblemBrief is the minimum problem card shown on a 题单 row.
type ProblemBrief struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	TimeLimitMS int    `json:"time_limit_ms"`
	MemLimitMB  int    `json:"mem_limit_mb"`
	Visibility  string `json:"visibility"`
}

// canEditList: only the creator, setters and admins manage 题单.
func (s *Server) canEditList(c *gin.Context, l *model.ProblemList) bool {
	claims := auth.CurrentUser(c)
	if claims == nil {
		return false
	}
	return isSetterRole(claims.Role) || l.CreatedBy == claims.UserID
}

// listLists: every logged-in user sees all 题单 (site-wide public).
func (s *Server) listLists(c *gin.Context) {
	var lists []model.ProblemList
	s.DB.Order("id DESC").Limit(200).Find(&lists)
	out := make([]listSummary, 0, len(lists))
	for _, l := range lists {
		var n int64
		s.DB.Model(&model.ProblemListItem{}).Where("list_id = ?", l.ID).Count(&n)
		out = append(out, listSummary{ProblemList: l, ProblemCount: n})
	}
	c.JSON(200, out)
}

type saveListReq struct {
	Title       string `json:"title" binding:"required,max=200"`
	Description string `json:"description"`
	// ProblemIDs in display order; replaces the previous set entirely.
	ProblemIDs []uint          `json:"problem_ids"`
	Notes      map[uint]string `json:"notes"` // problemID -> note (optional)
}

func (s *Server) createList(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var req saveListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	l := &model.ProblemList{Title: req.Title, Description: req.Description, CreatedBy: claims.UserID}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(l).Error; err != nil {
			return err
		}
		return s.replaceItems(tx, l.ID, req)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": "create failed"})
		return
	}
	c.JSON(200, l)
}

// replaceItems rewrites the item rows for a list. Why delete-then-insert:
// 题单 are small (≤200 rows), and append-only diffs would complicate ordering.
func (s *Server) replaceItems(tx *gorm.DB, listID uint, req saveListReq) error {
	if err := tx.Delete(&model.ProblemListItem{}, "list_id = ?", listID).Error; err != nil {
		return err
	}
	for i, pid := range req.ProblemIDs {
		item := model.ProblemListItem{ListID: listID, ProblemID: pid, OrderIndex: i, Note: req.Notes[pid]}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) updateList(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid list id"})
		return
	}
	l := &model.ProblemList{}
	if err := s.DB.First(l, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "list not found"})
		return
	}
	if !s.canEditList(c, l) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	var req saveListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(l).Updates(map[string]any{
			"title": req.Title, "description": req.Description,
		}).Error; err != nil {
			return err
		}
		return s.replaceItems(tx, l.ID, req)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": "update failed"})
		return
	}
	c.JSON(200, l)
}

func (s *Server) deleteList(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid list id"})
		return
	}
	l := &model.ProblemList{}
	if err := s.DB.First(l, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "list not found"})
		return
	}
	if !s.canEditList(c, l) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.ProblemListItem{}, "list_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(l).Error
	})
	if err != nil {
		c.JSON(500, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// getList: full items with caller progress. Problems hidden from the caller
// (e.g. pending-review copies) are skipped, not 404 — a 题单 stays browsable.
func (s *Server) getList(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid list id"})
		return
	}
	l := &model.ProblemList{}
	if err := s.DB.First(l, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "list not found"})
		return
	}
	var items []model.ProblemListItem
	s.DB.Where("list_id = ?", id).Order("order_index").Find(&items)

	claims := auth.CurrentUser(c)
	out := make([]listItemView, 0, len(items))
	for _, it := range items {
		prob := &model.Problem{}
		if err := s.DB.First(prob, it.ProblemID).Error; err != nil || !s.canSeeProblem(c, prob) {
			continue
		}
		out = append(out, listItemView{
			Item: it,
			ProblemBrief: ProblemBrief{
				ID: prob.ID, Title: prob.Title,
				TimeLimitMS: prob.TimeLimitMS, MemLimitMB: prob.MemLimitMB,
				Visibility: prob.Visibility,
			},
			Progress: s.listProgress(claims, prob.ID),
		})
	}
	editable := s.canEditList(c, l)
	c.JSON(200, gin.H{"list": l, "items": out, "editable": editable})
}

// listProgress: ac when any AC submission exists, tried when any other
// submission exists, todo otherwise. Cancelled submissions don't count —
// the jury nullified them, same rule as standings.
func (s *Server) listProgress(claims *auth.Claims, problemID uint) string {
	if claims == nil {
		return "todo"
	}
	var ac, tried int64
	s.DB.Model(&model.Submission{}).
		Where("user_id = ? AND problem_id = ? AND status = ? AND cancelled = ?", claims.UserID, problemID, model.SubAC, false).
		Limit(1).Count(&ac)
	if ac > 0 {
		return "ac"
	}
	s.DB.Model(&model.Submission{}).
		Where("user_id = ? AND problem_id = ? AND cancelled = ?", claims.UserID, problemID, false).
		Limit(1).Count(&tried)
	if tried > 0 {
		return "tried"
	}
	return "todo"
}
