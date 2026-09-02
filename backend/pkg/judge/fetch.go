package judge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const maxBlobSize = 256 << 20

// fetchClient downloads testdata blobs from the API server with the daemon
// token, caching them on disk by sha256 so repeat judging costs no network.
type fetchClient struct {
	HTTP  *http.Client
	token string
}

func (f *fetchClient) fetchBlob(ctx context.Context, base, token, blobDir string,
	problemID uint64, index int, kind, sha string, size int64) ([]byte, error) {
	dst := filepath.Join(blobDir, sha)
	if raw, err := os.ReadFile(dst); err == nil {
		return raw, nil
	}
	// why the /api/v1 prefix: FetchBase is the API server ROOT (as deployed),
	// and the internal testdata routes live under the versioned API group.
	url := fmt.Sprintf("%s/api/v1/internal/testdata/%d/%d/%s", base, problemID, index, kind)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := f.HTTP
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBlobSize))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != sha {
		return nil, fmt.Errorf("sha mismatch for case %d %s", index, kind)
	}
	if size > 0 && int64(len(raw)) != size {
		return nil, fmt.Errorf("size mismatch for case %d %s", index, kind)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return raw, nil // cache write is best-effort; bytes are still valid
	}
	_ = os.Rename(tmp, dst)
	return raw, nil
}
