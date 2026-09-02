// Package plugin — small shared helpers so hook/adapter files stay lean.
package plugin

import (
	"encoding/json"
	"io"
	"net/url"
)

// jsonMarshal wraps encoding/json so hook files read cleaner.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// drainAndClose consumes a small response body so the connection can be
// reused (webhook fire-and-forget does not read the payload).
func drainAndClose(resp io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp, 1<<16))
	_ = resp.Close()
}

// hostOf extracts the hostname from an URL for the SSRF guard.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}