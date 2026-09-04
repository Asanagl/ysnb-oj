// Package handler — admin log viewer: exposes recent journald entries from
// the oj-api and oj-judge systemd units to the admin console.
//
// Security model (read before extending):
//   - Route sits behind the admin-tier role guard (admin/super_admin), same
//     as user management — logs can contain usernames and error details.
//   - The reader process is started from a completely request-independent
//     argv (logViewerArgv: literals and strconv of package constants).
//     Unit selection and the search term are applied in Go, after
//     retrieval. The child is built as an explicit &exec.Cmd struct (same
//     pattern as pkg/sandbox's stage2 launcher): argv list, no shell, and
//     no byte of the HTTP request can reach the child process.
//     admin_logs_test.go pins these properties (constant argv, no shell
//     tokens) so future edits cannot silently regress.
//   - The oj service user needs journald read access: add it to the
//     `systemd-journal` group (one ops step, docs/deploy.md §日志查看器).
//
// Output format: `journalctl -o json` (one JSON object per line, the only
// line-oriented JSON mode journald offers). systemd 247 on Debian 11 lacks
// --output-fields, so we parse full records and extract just the four
// fields we surface; MESSAGE is embedded as a JSON string, so newline
// inside messages stays escaped and one-record-per-line parsing is safe.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Log viewer tunables; request parameters are clamped server-side and used
// only AFTER retrieval (Go-side filtering/slicing), never in the argv.
const (
	logViewerMaxLines = 2000
	logViewerTimeout  = 10 * time.Second
	logViewerMaxHours = 168
	logViewerDefHours = 24
	logViewerDefLines = 200

	// journalctlBinary is resolved once via PATH; Debian/Ubuntu/Arch all
	// ship journalctl in /usr/bin, and we only ever pass it literal flags.
	journalctlBinary = "journalctl"
)

type logEntry struct {
	TS    string `json:"ts"`
	Unit  string `json:"unit"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// journalRecord models the subset of journald JSON fields we read.
// MESSAGE can be a string or, when journald deems the value non-printable
// (e.g. binary payloads), an array of byte values — accept both.
type journalRecord struct {
	Unit     string          `json:"_SYSTEMD_UNIT"`
	Priority string          `json:"PRIORITY"`
	Realtime string          `json:"__REALTIME_TIMESTAMP"`
	Message  json.RawMessage `json:"MESSAGE"`
}

// adminLogs handles GET /admin/logs?unit=api|judge|all&q=...&lines=N&hours=H
func (s *Server) adminLogs(c *gin.Context) {
	unitKey := c.DefaultQuery("unit", "all")
	if unitKey != "api" && unitKey != "judge" && unitKey != "all" {
		c.JSON(400, gin.H{"error": "unit 必须是 api / judge / all"})
		return
	}
	hours := clampInt(atoiDefault(c.Query("hours"), logViewerDefHours), 1, logViewerMaxHours)
	lines := clampInt(atoiDefault(c.Query("lines"), logViewerDefLines), 1, logViewerMaxLines)

	ctx, cancel := context.WithTimeout(c.Request.Context(), logViewerTimeout)
	defer cancel()
	out, errOut, err := runJournalctl(ctx)
	if err != nil {
		// exit 1 with empty output = "no entries matched" on journalctl;
		// anything else (missing binary, permission) is a real error.
		if ee, ok := err.(*exec.ExitError); ok && out.Len() == 0 && ee.ExitCode() == 1 {
			slog.Warn("journalctl exited 1", "stderr", errOut.String())
			c.JSON(200, gin.H{"entries": []logEntry{}})
			return
		}
		slog.Error("journalctl read failed", "err", err, "stderr", errOut.String())
		c.JSON(500, gin.H{"error": "journalctl 读取失败（检查 oj 用户是否在 systemd-journal 组）"})
		return
	}
	entries := parseJournalJSON(out.Bytes())
	// INFO (not debug): one line per viewer query is cheap and invaluable
	// when diagnosing journald/permission issues remotely.
	slog.Info("journalctl read", "bytes", out.Len(), "entries", len(entries))
	if unitKey != "all" {
		want := "oj-api.service"
		if unitKey == "judge" {
			want = "oj-judge.service"
		}
		filtered := entries[:0:0]
		for _, e := range entries {
			if e.Unit == want {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	if q := strings.ToLower(strings.TrimSpace(c.Query("q"))); q != "" {
		filtered := entries[:0:0]
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Msg), q) ||
				strings.Contains(strings.ToLower(e.Unit), q) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	if len(entries) > lines {
		entries = entries[len(entries)-lines:] // keep the most recent N
	}
	c.JSON(200, gin.H{"entries": entries, "window_hours": hours, "max_lines": lines})
}

// logViewerArgv is the immutable journalctl argument list: only literals
// and strconv of package constants. It never varies with the request, so
// the child command shape is request-independent (no injection surface).
// Both units are always included; unit filtering happens in Go so the argv
// stays identical for every request. NOTE: no --since here — combining
// --since with --lines takes the FIRST N records after the window start
// (i.e. the oldest); --lines alone yields the most recent N, which is what
// the viewer wants. Output is still oldest→newest within that set.
func logViewerArgv() []string {
	return []string{
		"--no-pager",
		"-o", "json",
		"--unit", "oj-api.service",
		"--unit", "oj-judge.service",
		"--lines", strconv.Itoa(logViewerMaxLines),
	}
}

// runJournalctl starts the reader process and collects its stdout. The
// command is assembled as an explicit &exec.Cmd (argv list, no shell) from
// the constant logViewerArgv — the same construction the sandbox uses for
// its stage2 launcher. LookPath only searches PATH for the binary name; it
// performs no execution. ctx deadline is wired manually because a literal
// &exec.Cmd has no CommandContext hook.
func runJournalctl(ctx context.Context) (*bytes.Buffer, *bytes.Buffer, error) {
	path, err := exec.LookPath(journalctlBinary)
	if err != nil {
		return nil, nil, err
	}
	var out, errOut bytes.Buffer
	cmd := &exec.Cmd{
		Path:   path,
		Args:   append([]string{path}, logViewerArgv()...),
		Stdout: &out,
		Stderr: &errOut,
	}
	if err := cmd.Start(); err != nil {
		return nil, &errOut, err
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		return &out, &errOut, err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-waitErr
		return &out, &errOut, ctx.Err()
	}
}

// parseJournalJSON reads `journalctl -o json` output: one JSON object per
// line. MESSAGE values are usually JSON strings; journald may also encode
// non-printable payloads as byte arrays — those are rendered as a visible
// placeholder (byte values are not useful in the UI). Records without a
// usable MESSAGE are skipped. Malformed lines are skipped too.
func parseJournalJSON(out []byte) []logEntry {
	entries := make([]logEntry, 0, 64)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		msg, err := decodeMessage(rec.Message)
		if err != nil || msg == "" {
			continue
		}
		entries = append(entries, logEntry{
			TS:    epochMicroToRFC3339(rec.Realtime),
			Unit:  rec.Unit,
			Level: priorityToLevel(rec.Priority),
			Msg:   msg,
		})
	}
	return entries
}

// decodeMessage converts the MESSAGE raw JSON value into display text:
// string → as-is; byte array → "«binary N bytes»"; anything else → error.
func decodeMessage(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", fmt.Errorf("empty message")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var arr []int
	if err := json.Unmarshal(raw, &arr); err == nil {
		return fmt.Sprintf("«binary record, %d bytes»", len(arr)), nil
	}
	return "", fmt.Errorf("unsupported message encoding")
}

func priorityToLevel(p string) string {
	switch p {
	case "0", "1", "2", "3":
		return "error"
	case "4":
		return "warn"
	case "5", "6":
		return "info"
	default:
		return "debug"
	}
}

func epochMicroToRFC3339(us string) string {
	n, err := strconv.ParseInt(us, 10, 64)
	if err != nil {
		return us
	}
	return time.UnixMicro(n).Format("2006-01-02 15:04:05.000")
}
