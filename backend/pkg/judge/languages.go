// Package judge orchestrates one submission through compile → per-case run →
// compare, using the sandbox for every process it starts (user code, checker,
// interactor alike). Language support is data-driven from languages.yaml.
package judge

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed languages.yaml
var defaultLanguages []byte

type Language struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	// SourceFile is the on-disk name the code is written to; Java requires
	// the public class name to match, so it is part of the toolchain contract.
	SourceFile string          `yaml:"source_file" json:"source_file"`
	Compile    *CompileProfile `yaml:"compile" json:"compile,omitempty"`
	Run        RunProfile      `yaml:"run" json:"run"`
}

type CompileProfile struct {
	Argv     []string `yaml:"argv" json:"argv"`
	TimeoutS int64    `yaml:"timeout_s" json:"timeout_s"`
	MemMB    int64    `yaml:"mem_mb" json:"mem_mb"`
}

type RunProfile struct {
	Argv           []string `yaml:"argv" json:"argv"`
	TimeMultiplier float64  `yaml:"time_multiplier" json:"time_multiplier"`
	MemOverheadMB  int64    `yaml:"mem_overhead_mb" json:"mem_overhead_mb"`
	StackKB        int64    `yaml:"stack_kb" json:"stack_kb"`
	AsLimited      bool     `yaml:"as_limited" json:"as_limited"`
}

// Registry loads languages.yaml (embedded default, optional file override)
// and serves read-only lookups; Registries are safe for concurrent use.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]*Language
	list []*Language
}

// NewRegistry builds from the embedded defaults; overridePath (optional)
// replaces entries sharing an id or adds new ones. The override path comes
// from the service operator (OJ_LANGUAGES env / --languages flag) — a
// principal that owns the process context; the .yaml-only + no-traversal
// check guards against accidents (wrong env var) and documents the trust
// boundary, not against an attacker who already controls the environment.
func NewRegistry(overridePath string) (*Registry, error) {
	r := &Registry{byID: map[string]*Language{}}
	if err := r.loadInto(defaultLanguages); err != nil {
		return nil, fmt.Errorf("embedded languages.yaml: %w", err)
	}
	if overridePath != "" {
		if !strings.HasSuffix(overridePath, ".yaml") && !strings.HasSuffix(overridePath, ".yml") {
			return nil, fmt.Errorf("language override %q: only .yaml/.yml accepted", overridePath)
		}
		clean := filepath.ToSlash(filepath.Clean(overridePath))
		for _, seg := range strings.Split(clean, "/") {
			if seg == ".." {
				return nil, fmt.Errorf("language override %q: traversal segments not allowed", overridePath)
			}
		}
		raw, err := os.ReadFile(overridePath)
		if err != nil {
			return nil, fmt.Errorf("read language override: %w", err)
		}
		if err := r.loadInto(raw); err != nil {
			return nil, fmt.Errorf("language override %s: %w", overridePath, err)
		}
	}
	return r, nil
}

type languageFile struct {
	Languages []*Language `yaml:"languages"`
}

func (r *Registry) loadInto(raw []byte) error {
	var parsed languageFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	for _, lang := range parsed.Languages {
		if lang.ID == "" || lang.SourceFile == "" || len(lang.Run.Argv) == 0 {
			return fmt.Errorf("language %q missing id/source_file/run.argv", lang.ID)
		}
		if existing, ok := r.byID[lang.ID]; ok {
			*r.list[indexOf(r.list, existing)] = *lang
		} else {
			r.list = append(r.list, lang)
			r.byID[lang.ID] = lang
		}
	}
	return nil
}

func indexOf(list []*Language, target *Language) int {
	for i, l := range list {
		if l == target {
			return i
		}
	}
	return -1
}

func (r *Registry) Get(id string) (*Language, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lang, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("unknown language %q", id)
	}
	return lang, nil
}

func (r *Registry) List() []*Language {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Language, len(r.list))
	copy(out, r.list)
	return out
}

// BinCache caches compiled artifacts (user binaries are per-submission, but
// checker/interactor binaries are per-problem and worth persisting by hash).
type BinCache struct {
	Root string
	mu   sync.Mutex
}

func NewBinCache(root string) (*BinCache, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &BinCache{Root: root}, nil
}

func (c *BinCache) path(key string) string {
	return filepath.Join(c.Root, key+".bin")
}

// Get returns the cached artifact path when present.
func (c *BinCache) Get(key string) (string, bool) {
	p := c.path(key)
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
		return p, true
	}
	return "", false
}

// Put atomically installs an artifact so concurrent judges never observe a
// half-written binary.
func (c *BinCache) Put(key, srcPath string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dst := c.path(key)
	tmp := dst + ".tmp"
	if err := copyFile(srcPath, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o755)
}
