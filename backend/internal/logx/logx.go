// Package logx is the project-wide structured logging layer on top of
// log/slog. Everything goes to stderr as single-line JSON so journald
// indexes each attribute (PRIORITY picks up from the level) and
// `journalctl -u oj-api -o json` stays machine-greppable.
//
// Level policy: INFO is the default production level (one summary line per
// submission, per dispatch error); DEBUG adds per-case judge detail and is
// enabled via OJ_LOG_LEVEL=debug without recompiling.
package logx

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel maps OJ_LOG_LEVEL (debug|info|warn|error, default info) to a
// slog.Level; unknown values fall back to INFO with the value logged once.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init installs the global JSON logger and returns it. Both api and judge
// call this once at startup; after that any package can use slog.Default().
func Init(level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		// ReplaceAttr maps slog levels onto syslog priorities journald
		// understands, so `journalctl -p err` filters without grep.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey && len(groups) == 0 {
				if lv, ok := a.Value.Any().(slog.Level); ok {
					switch {
					case lv >= slog.LevelError:
						a.Value = slog.StringValue("err")
					case lv >= slog.LevelWarn:
						a.Value = slog.StringValue("warning")
					case lv >= slog.LevelInfo:
						a.Value = slog.StringValue("info")
					default:
						a.Value = slog.StringValue("debug")
					}
				}
			}
			return a
		},
	})
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

// Discard installs a no-op logger (selftest paths, tools).
func Discard() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 4})))
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// Debug/Info/Warn/Error are thin wrappers so call sites stay one-liners
// with the component as the first field.
func Debug(ctx context.Context, msg string, args ...any) { slog.Default().DebugContext(ctx, msg, args...) }
func Info(ctx context.Context, msg string, args ...any)  { slog.Default().InfoContext(ctx, msg, args...) }
func Warn(ctx context.Context, msg string, args ...any)  { slog.Default().WarnContext(ctx, msg, args...) }
func Error(ctx context.Context, msg string, args ...any) { slog.Default().ErrorContext(ctx, msg, args...) }
