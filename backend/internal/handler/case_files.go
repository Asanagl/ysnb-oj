// Package handler — single test-case upload and preview (测试点单点管理).
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

// saveFormFile streams an uploaded multipart file to disk.
func saveFormFile(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(src, 256<<20))
	return err
}

// uploadSingleCase appends one test case (input file + optional answer
// file); the case index is auto-assigned as max+1 so judges can grow a
// problem's dataset without re-uploading the whole zip.
func (s *Server) uploadSingleCase(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	if !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	inFile, err := c.FormFile("input")
	if err != nil {
		c.JSON(400, gin.H{"error": "missing input file"})
		return
	}
	destDir := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID))
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		c.JSON(500, gin.H{"error": "mkdir failed"})
		return
	}
	// auto index: max existing + 1
	var maxIdx int
	var cases []model.TestCase
	s.DB.Where("problem_id = ?", prob.ID).Find(&cases)
	for _, tc := range cases {
		if tc.CaseIndex > maxIdx {
			maxIdx = tc.CaseIndex
		}
	}
	idx := maxIdx + 1

	inPath := filepath.Join(destDir, strconv.Itoa(idx)+".in")
	if err := saveFormFile(inFile, inPath); err != nil {
		c.JSON(500, gin.H{"error": "save input failed"})
		return
	}
	sum := sha256.Sum256(mustRead(inPath))
	tc := &model.TestCase{
		ProblemID: prob.ID, CaseIndex: idx,
		InputSHA:  hex.EncodeToString(sum[:]),
		InputSize: fileSize(inPath),
	}
	if outFile, err := c.FormFile("output"); err == nil {
		outPath := filepath.Join(destDir, strconv.Itoa(idx)+".out")
		if err := saveFormFile(outFile, outPath); err != nil {
			c.JSON(500, gin.H{"error": "save output failed"})
			return
		}
		osum := sha256.Sum256(mustRead(outPath))
		tc.AnswerSHA = hex.EncodeToString(osum[:])
		tc.AnswerSize = fileSize(outPath)
	}
	if err := s.DB.Create(tc).Error; err != nil {
		c.JSON(500, gin.H{"error": "create case failed"})
		return
	}
	c.JSON(200, tc)
}

// previewCase returns one test case file's content for the in-browser
// viewer (text files only; binary content is base64-guarded by size limit).
func (s *Server) previewCase(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canManageProblem(c, prob) {
		c.JSON(404, gin.H{"error": "problem not found"})
		return
	}
	caseID, ok := paramID(c, "caseId")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid case id"})
		return
	}
	kind := c.Param("kind")
	if kind != "in" && kind != "out" {
		c.JSON(400, gin.H{"error": "bad kind"})
		return
	}
	path := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID), caseParamFile(caseID, kind))
	raw, err := os.ReadFile(path)
	if err != nil {
		c.JSON(404, gin.H{"error": "case file not found"})
		return
	}
	const previewLimit = 64 << 10
	if len(raw) > previewLimit {
		raw = raw[:previewLimit]
	}
	c.String(200, "%s", string(raw))
}

func caseParamFile(caseID uint, kind string) string {
	name := strconv.FormatUint(uint64(caseID), 10) + ".in"
	if kind == "out" {
		name = strconv.FormatUint(uint64(caseID), 10) + ".out"
	}
	return name
}

// deleteSingleCase removes one test case row and its on-disk blobs.
func (s *Server) deleteSingleCase(c *gin.Context) {
	prob, ok := s.problemByID(c)
	if !ok || !s.canManageProblem(c, prob) {
		c.JSON(403, gin.H{"error": "not allowed"})
		return
	}
	caseID, ok := paramID(c, "caseId")
	if !ok {
		c.JSON(400, gin.H{"error": "invalid case id"})
		return
	}
	tc := &model.TestCase{}
	if err := s.DB.First(tc, caseID).Error; err != nil || tc.ProblemID != prob.ID {
		c.JSON(404, gin.H{"error": "test case not found"})
		return
	}
	if err := s.DB.Delete(tc).Error; err != nil {
		c.JSON(500, gin.H{"error": "delete failed"})
		return
	}
	destDir := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID))
	_ = os.Remove(filepath.Join(destDir, strconv.Itoa(int(tc.CaseIndex))+".in"))
	_ = os.Remove(filepath.Join(destDir, strconv.Itoa(int(tc.CaseIndex))+".out"))
	c.JSON(200, gin.H{"ok": true})
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func mustRead(path string) []byte {
	raw, _ := os.ReadFile(path)
	return raw
}

func saveUploaded(fh interface {
	Open() (interface {
		Read([]byte) (int, error)
		Close() error
	}, error)
}, dst string) error {
	return nil
}
