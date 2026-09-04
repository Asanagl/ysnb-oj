// Package plugin — built-in event hooks. The webhook hook delivers judge
// lifecycle events to operator-configured HTTP endpoints: the QQ bot
// (qqcot) and any dashboard can subscribe without touching core code.
// Delivery is best-effort POST JSON with a short timeout; failures are
// logged and dropped (hooks are observability, not a queue).
package plugin

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// parseURL wraps url.Parse so ValidateHookURL stays on one line.
func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func init() { RegisterEventHook(newWebhookHook()) }

// WebhookHook is the exported handle so the admin handler can manage
// targets through the registry (EventHookByID → *WebhookHook).
type WebhookHook = webhookHook

// webhookHook holds admin-configured target URLs. Targets are runtime
// configuration (POST /admin/plugins/hooks), not compile-time constants.
type webhookHook struct {
	mu      sync.RWMutex
	targets map[string]string // name -> URL
	client  *http.Client
}

func newWebhookHook() *webhookHook {
	return &webhookHook{
		targets: map[string]string{},
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (w *webhookHook) Name() string { return "webhook" }

// ValidateHookURL checks a webhook target before it is stored: http/https
// only and host must not resolve to a local/private/reserved address (the
// SSRF rule every outbound URL in this project must pass).
func ValidateHookURL(urlText string) error {
	u, err := parseURL(urlText)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("仅允许 http/https")
	}
	return SafeResolve(u.Hostname())
}

// SetTarget upserts a named endpoint; empty url removes it.
func (w *webhookHook) SetTarget(name, url string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if url == "" {
		delete(w.targets, name)
		return
	}
	w.targets[name] = url
}

func (w *webhookHook) Targets() map[string]string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make(map[string]string, len(w.targets))
	for k, v := range w.targets {
		out[k] = v
	}
	return out
}

func (w *webhookHook) OnJudgeEvent(event map[string]any) {
	w.mu.RLock()
	targets := make([]string, 0, len(w.targets))
	for _, u := range w.targets {
		targets = append(targets, u)
	}
	w.mu.RUnlock()
	if len(targets) == 0 {
		return
	}
	body, err := jsonMarshal(event)
	if err != nil {
		return
	}
	for _, url := range targets {
		if err := w.postOnce(url, body); err != nil {
			slog.Warn("plugin webhook delivery failed", "url", url, "err", err)
		}
	}
}

func (w *webhookHook) postOnce(url string, body []byte) error {
	if err := SafeResolve(hostOf(url)); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	drainAndClose(resp.Body)
	return nil
}