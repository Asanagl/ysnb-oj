// Package handler — external practice sync worker: pulls bound users'
// submission logs from every registered plugin.SubmitLogFetcher platform on
// a ticker (plus on-demand syncs from the report page), de-duplicating by
// the platform-native submission id. Started from cmd/api/main.go; a failed
// platform sync logs and continues — one flaky site must not stall others.
package handler

import (
	"context"
	"log"
	"time"

	"github.com/ysnb/oj/internal/plugin"
)

func pluginSubmitFetcherExists(name string) bool {
	_, ok := plugin.SubmitFetcherByID(name)
	return ok
}

// syncBinding pulls (needAll on first bind; incremental afterwards) and
// stores new rows. Returns (stored, lastError). Dedup relies on the unique
// index over external_id: platform + ":" + platform submission id.
func (s *Server) syncBinding(b *ExternalBinding, first bool) (int, string) {
	fetcher, ok := plugin.SubmitFetcherByID(b.Platform)
	if !ok {
		return 0, "平台插件未注册: " + b.Platform
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	recs, err := fetcher.FetchSubmitLog(ctx, b.Handle, first)
	if err != nil {
		msg := err.Error()
		if len(msg) > 480 {
			msg = msg[:480]
		}
		s.DB.Model(b).Update("last_error", msg)
		return 0, msg
	}
	stored := 0
	for _, r := range recs {
		row := &ExternalRecord{
			UserID: b.UserID, Platform: b.Platform,
			ExternalID: b.Platform + ":" + r.ExternalID,
			ProblemID:  r.ProblemID, ProblemName: r.ProblemName,
			Verdict: r.Verdict, Language: r.Language, At: r.At,
			FetchedAt: time.Now(),
		}
		if s.DB.Create(row).Error == nil {
			stored++
		}
	}
	now := time.Now()
	s.DB.Model(b).Updates(map[string]any{"synced_at": now, "last_error": ""})
	return stored, ""
}

// StartExternalSyncScanner ticks hourly: incremental sync for every binding.
// Started from cmd/api/main.go next to the requeue scanner.
func (s *Server) StartExternalSyncScanner(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.syncAllBindings(ctx)
			}
		}
	}()
}

// syncAllBindings walks every binding sequentially — crawl politeness: the
// point of the hourly scan is freshness, not parallel hammering.
func (s *Server) syncAllBindings(ctx context.Context) {
	var bindings []ExternalBinding
	if err := s.DB.Find(&bindings).Error; err != nil {
		log.Printf("[external-sync] list bindings: %v", err)
		return
	}
	for i := range bindings {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if _, lastErr := s.syncBinding(&bindings[i], false); lastErr != "" {
			log.Printf("[external-sync] %s/%s: %s", bindings[i].Platform, bindings[i].Handle, lastErr)
		}
		// space sequential platform hits; rate-limits are per-source
		time.Sleep(2 * time.Second)
	}
}