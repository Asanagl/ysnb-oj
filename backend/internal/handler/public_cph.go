// Package handler — cph bridge + keyed sample endpoints for the public API.
//
// cph (Competitive Programming Helper, VSCode) has no remote problem-source
// protocol; it consumes (a) a local "import problem" JSON document and (b)
// the Competitive Companion browser-extension push (an HTTP POST to
// 127.0.0.1:4244 with a JSON problem object). This file provides:
//   - GET  /public/problems/:id/cph        → cph import JSON (keyed)
//   - GET  /public/problems/:id/samples    → statement samples (keyed)
// Both hand out machine-consumable assets, hence the API-key line.
// Full judging testdata (.in/.out sets) is NEVER exposed here.
package handler

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

type cphSample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// cphProblem mirrors the JSON that cph's "Import from JSON" accepts
// (cphProblem meta.json shape): name, group, url, tests[{input,output}],
// and optional interactive flag.
type cphProblem struct {
	Name       string      `json:"name"`
	Group      string      `json:"group"`
	URL        string      `json:"url,omitempty"`
	Interactive bool       `json:"interactive,omitempty"`
	MemoryLimit int        `json:"memoryLimit"` // MB
	TimeLimit  int         `json:"timeLimit"`   // ms
	Tests      []cphSample `json:"tests"`
}

// publicProblemSamples serves GET /public/problems/:id/samples — the
// statement's public samples as structured JSON (the judging testdata set
// is deliberately never reachable through the public API).
func (s *Server) publicProblemSamples(c *gin.Context) {
	prob, ok := s.publicVisibleProblem(c)
	if !ok {
		return
	}
	samples := []model.Sample{}
	_ = json.Unmarshal([]byte(prob.Samples), &samples)
	c.JSON(200, gin.H{
		"problem_id": prob.ID, "title": prob.Title,
		"time_limit_ms": prob.TimeLimitMS, "mem_limit_mb": prob.MemLimitMB,
		"samples": samples,
	})
}

// publicProblemCph serves GET /public/problems/:id/cph — a ready-to-import
// cph problem JSON (name/group/url/tests + limits). Keyed endpoint: it
// packages the statement samples into the exact shape cph imports.
func (s *Server) publicProblemCph(c *gin.Context) {
	prob, ok := s.publicVisibleProblem(c)
	if !ok {
		return
	}
	samples := []model.Sample{}
	_ = json.Unmarshal([]byte(prob.Samples), &samples)
	tests := make([]cphSample, 0, len(samples))
	for _, s := range samples {
		tests = append(tests, cphSample{Input: s.Input, Output: s.Output})
	}
	c.JSON(200, cphProblem{
		Name: prob.Title, Group: "YSNB OJ",
		URL: publicBaseURL(c) + "/problems/" + strconv.FormatUint(uint64(prob.ID), 10),
		MemoryLimit: prob.MemLimitMB, TimeLimit: prob.TimeLimitMS,
		Tests: tests,
	})
}

// publicVisibleProblem loads a problem that is bank-visible to anonymous
// callers (not hidden, not contest-exclusive). Contest copies and hidden
// problems 404 here — their samples travel only inside a live contest.
func (s *Server) publicVisibleProblem(c *gin.Context) (*model.Problem, bool) {
	if !requireAPIKey(c) {
		return nil, false
	}
	id, ok := paramID(c, "id")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid id"})
		return nil, false
	}
	prob := &model.Problem{}
	if err := s.DB.First(prob, id).Error; err != nil ||
		prob.ContestID != nil || prob.Visibility == model.VisibilityHidden {
		c.JSON(404, gin.H{"error": "problem not found"})
		return nil, false
	}
	return prob, true
}

func publicBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}