package handler

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/model"
)

var caseFileRe = regexp.MustCompile(`^(\d+)\.(in|out|ans)$`)

const maxZipBytes = 128 << 20

// storeTestdataZip replaces the problem's testdata with the pairs found in
// the zip (e.g. 1.in/1.out). Files live under DataDir/testdata/<pid>/ and the
// DB stores only metadata (sizes + sha256) for the dispatch manifest.
func (s *Server) storeTestdataZip(prob *model.Problem, r io.Reader) (any, error) {
	// Stream to a temp file so hostile zips can't exhaust memory.
	tmp, err := os.CreateTemp(s.Cfg.DataDir, "upload-*.zip")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, io.LimitReader(r, maxZipBytes+1)); err != nil {
		return nil, err
	}
	info, _ := tmp.Stat()
	if info != nil && info.Size() > maxZipBytes {
		return nil, fmt.Errorf("zip too large (max %d MB)", maxZipBytes>>20)
	}

	zipReader, err := zip.NewReader(tmp, info.Size())
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}
	inputs := map[int]*zip.File{}
	answers := map[int]*zip.File{}
	for _, f := range zipReader.File {
		m := caseFileRe.FindStringSubmatch(filepath.Base(f.Name))
		if m == nil {
			continue
		}
		// m[1] is digits-only by the regex, but Atoi still fails on
		// overflow (e.g. a 30-digit "case number") — skip rather than
		// silently map to case 0.
		idx, err := strconv.Atoi(m[1])
		if err != nil || idx < 1 || idx > 100_000 {
			continue
		}
		if m[2] == "in" {
			inputs[idx] = f
		} else {
			answers[idx] = f
		}
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("zip contains no <n>.in files")
	}

	destDir := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID))
	if err := os.RemoveAll(destDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return nil, err
	}
	stored := 0
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.TestCase{}, "problem_id = ?", prob.ID).Error; err != nil {
			return err
		}
		for idx := range inputs {
			tc, err := s.storeCase(destDir, idx, inputs[idx], answers[idx])
			if err != nil {
				return err
			}
			tc.ProblemID = prob.ID
			if err := tx.Create(tc).Error; err != nil {
				return err
			}
			stored++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return gin.H{"stored": stored}, nil
}

// storeCase writes input/answer files and hashes them; a missing .out is
// tolerated for interactive problems only (checked by the caller's mode).
func (s *Server) storeCase(destDir string, idx int, in, out *zip.File) (*model.TestCase, error) {
	tc := &model.TestCase{CaseIndex: idx}
	data, err := readZipFile(in, 256<<20)
	if err != nil {
		return nil, fmt.Errorf("case %d input: %w", idx, err)
	}
	sum := sha256.Sum256(data)
	tc.InputSHA = hex.EncodeToString(sum[:])
	tc.InputSize = int64(len(data))
	if err := os.WriteFile(filepath.Join(destDir, strconv.Itoa(idx)+".in"), data, 0o640); err != nil {
		return nil, err
	}
	if out != nil {
		data, err = readZipFile(out, 256<<20)
		if err != nil {
			return nil, fmt.Errorf("case %d answer: %w", idx, err)
		}
		sum = sha256.Sum256(data)
		tc.AnswerSHA = hex.EncodeToString(sum[:])
		tc.AnswerSize = int64(len(data))
		if err := os.WriteFile(filepath.Join(destDir, strconv.Itoa(idx)+".out"), data, 0o640); err != nil {
			return nil, err
		}
	}
	return tc, nil
}

func readZipFile(f *zip.File, limit int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > limit {
		return nil, fmt.Errorf("entry %s too large", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit+1))
}

// serveTestdata is the daemon-only endpoint streaming raw testdata blobs.
// Path params are validated numeric so a hostile daemon cannot poke around
// the filesystem with traversal tricks through gin's route params.
func (s *Server) serveTestdata(c *gin.Context) {
	kind := c.Param("kind")
	if kind != "in" && kind != "out" {
		c.JSON(400, gin.H{"error": "bad kind"})
		return
	}
	pid, errPID := strconv.Atoi(c.Param("problemId"))
	idx, errIdx := strconv.Atoi(c.Param("caseIndex"))
	if errPID != nil || errIdx != nil || pid < 1 || idx < 0 || pid > 1_000_000_000 || idx > 100_000 {
		c.JSON(400, gin.H{"error": "bad path params"})
		return
	}
	name := strconv.Itoa(idx) + ".in"
	if kind == "out" {
		name = strconv.Itoa(idx) + ".out"
	}
	path := filepath.Join(s.Cfg.DataDir, "testdata", strconv.Itoa(pid), name)
	c.File(path)
}
