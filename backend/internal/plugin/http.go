// Package plugin — HTTP helper for external-platform adapters: a single
// client with browser-like headers, hard timeouts, size caps and the
// SSRF guard (http/https only, host resolved and rejected when it points at
// localhost / loopback / private / reserved ranges). Every outbound fetch
// from a plugin must go through this — platform URLs arrive from user
// config or crawler results and are attacker-adjacent input.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

var errUnsafeHost = errors.New("plugin: host resolved to a local/private/reserved address")

const maxFetchBytes = 8 << 20 // external pages are metadata, not testdata

// SafeHTTPGet issues a GET against urlText with SSRF validation and returns
// the body (capped). redirects are followed only through SafeResolve.
func SafeHTTPGet(ctx context.Context, urlText string) ([]byte, error) {
	u, err := url.Parse(urlText)
	if err != nil {
		return nil, fmt.Errorf("plugin: bad url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("plugin: scheme %q not allowed", u.Scheme)
	}
	if err := SafeResolve(u.Hostname()); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlText, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; YSNB-OJ-sync/1.0)")
	client := &http.Client{
		Timeout: 15 * time.Second,
		// every hop re-validates: a redirect must not smuggle us inward
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := SafeResolve(req.URL.Hostname()); err != nil {
				return err
			}
			if len(via) >= 5 {
				return errors.New("plugin: too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin: %s -> HTTP %d", urlText, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
}

// SafeResolve rejects hostnames whose DNS answers with private/loopback/
// link-local/reserved addresses, and literal IPs of the same families.
func SafeResolve(host string) error {
	if host == "" {
		return errUnsafeHost
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("plugin: resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errUnsafeHost
		}
	}
	return nil
}