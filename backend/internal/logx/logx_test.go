package logx

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug, "DEBUG": slog.LevelDebug,
		"info": slog.LevelInfo, "": slog.LevelInfo, "junk": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestJSONOutputAndPriority(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Info("judge done", "submission", 42, "status", "AC")
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(`"msg":"judge done"`)) ||
		!bytes.Contains(buf.Bytes(), []byte(`"submission":42`)) {
		t.Fatalf("json output missing fields: %s", out)
	}
}

func TestDiscardSilences(t *testing.T) {
	Discard()
	// must not panic; Discard sets a level above everything
	slog.Default().Error("should be invisible")
}
