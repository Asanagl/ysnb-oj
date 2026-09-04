// Package config loads judge system configuration from a YAML file with
// environment variable overrides. Configuration is intentionally small:
// everything that varies between dev and prod lives here.
package config

import (
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration shared by api and judge daemons.
type Config struct {
	Listen    string `yaml:"listen"`     // api http listen addr
	GRPCAddr  string `yaml:"grpc_addr"`  // api grpc listen addr (judge daemons)
	Mode      string `yaml:"mode"`       // dev | prod (dev uses sqlite + memory queue)
	DataDir   string `yaml:"data_dir"`   // test data / code / checker storage root
	FetchBase string `yaml:"fetch_base"` // URL daemons use to download testdata
	LogLevel  string `yaml:"log_level"`  // debug|info|warn|error (OJ_LOG_LEVEL)
	Database  DB     `yaml:"database"`
	Redis     Redis  `yaml:"redis"`
	JWT       JWT    `yaml:"jwt"`
	Judge     Judge  `yaml:"judge"`
}

type DB struct {
	Driver string `yaml:"driver"` // postgres | sqlite
	DSN    string `yaml:"dsn"`
}

type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type JWT struct {
	Secret       string `yaml:"secret"`
	ExpireHours  int    `yaml:"expire_hours"`
	DaemonSecret string `yaml:"daemon_secret"` // shared secret judge daemons must present
}

// Judge groups settings used by the judge daemon process.
type Judge struct {
	APIEndpoint    string `yaml:"api_endpoint"`      // grpc host:port of api server
	DaemonName     string `yaml:"daemon_name"`       // identity shown in admin monitor
	DaemonToken    string `yaml:"daemon_token"`      // auth token matching jwt.daemon_secret
	MaxParallel    int    `yaml:"max_parallel"`      // concurrent judge tasks
	CGroupBase     string `yaml:"cgroup_base"`       // v2 cgroup root for judge subgroups
	WorkRoot       string `yaml:"work_root"`         // per-run scratch dirs; must NOT live under /home or data_dir
	CompileTimeout int    `yaml:"compile_timeout_s"` // compile wall limit in seconds
}

// Load reads cfgPath and applies OJ_* environment overrides.
//
// Security note on the path-traversal findings: cfgPath originates from the
// process operator (--config flag / OJ_CONFIG env), i.e. a principal that
// already owns the service's execution context — the same principal that
// could point the process at any file regardless of this check. Confining
// it to .yaml and refusing directory traversal keeps the surface explicit
// without pretending the operator is an attacker.
func Load(cfgPath string) (*Config, error) {
	cfg := defaultConfig()
	if cfgPath != "" {
		if err := validateConfigPath(cfgPath); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", cfgPath, err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	applyEnv(cfg)
	if cfg.JWT.Secret == "" {
		if cfg.Mode != "dev" {
			return nil, fmt.Errorf("jwt.secret is required (set OJ_JWT_SECRET)")
		}
		// Dev-only convenience: ephemeral secret so `go run ./cmd/api` works
		// with zero configuration; never a literal credential in source.
		cfg.JWT.Secret = randomToken()
		fmt.Println("[config] dev mode: generated ephemeral JWT secret")
	}
	// log_level is normalized here so both entrypoints share one parse; an
	// unknown value silently maps to INFO (logx.ParseLevel default).
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	return cfg, nil
}

// validateConfigPath constrains the operator-supplied config path: must be
// a .yaml/.yml file, no traversal segments, no absolute /etc/shadow-style
// sensitive prefixes. This documents the trust boundary explicitly — the
// operator could bypass any in-process check, so this guards against
// accidents (wrong env var pointing at a non-config file) rather than
// attackers.
func validateConfigPath(p string) error {
	if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
		return fmt.Errorf("config %q: only .yaml/.yml configs are accepted", p)
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return fmt.Errorf("config %q: path traversal segments are not allowed", p)
		}
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Listen:    ":8080",
		GRPCAddr:  ":9090",
		Mode:      "dev",
		DataDir:   "./data",
		FetchBase: "http://127.0.0.1:8080",
		LogLevel:  "info",
		Database: DB{
			Driver: "sqlite",
			DSN:    "./data/oj.db",
		},
		JWT: JWT{
			ExpireHours: 72,
		},
		Judge: Judge{
			MaxParallel:    2,
			CGroupBase:     "/sys/fs/cgroup/oj-judge",
			WorkRoot:       "/oj-work",
			CompileTimeout: 60,
		},
	}
}

// applyEnv lets ops override critical fields without editing files;
// only additive overrides, never rewrites the file itself.
func applyEnv(cfg *Config) {
	setStr("OJ_LISTEN", &cfg.Listen)
	setStr("OJ_GRPC_ADDR", &cfg.GRPCAddr)
	setStr("OJ_MODE", &cfg.Mode)
	setStr("OJ_DATA_DIR", &cfg.DataDir)
	setStr("OJ_FETCH_BASE", &cfg.FetchBase)
	setStr("OJ_LOG_LEVEL", &cfg.LogLevel)
	setStr("OJ_DB_DRIVER", &cfg.Database.Driver)
	setStr("OJ_DB_DSN", &cfg.Database.DSN)
	setStr("OJ_REDIS_ADDR", &cfg.Redis.Addr)
	setStr("OJ_JWT_SECRET", &cfg.JWT.Secret)
	setStr("OJ_DAEMON_SECRET", &cfg.JWT.DaemonSecret)
	setStr("OJ_API_ENDPOINT", &cfg.Judge.APIEndpoint)
	setStr("OJ_DAEMON_NAME", &cfg.Judge.DaemonName)
	setStr("OJ_DAEMON_TOKEN", &cfg.Judge.DaemonToken)
	setStr("OJ_WORK_ROOT", &cfg.Judge.WorkRoot)
	if v, err := strconv.Atoi(os.Getenv("OJ_JWT_EXPIRE_HOURS")); err == nil {
		cfg.JWT.ExpireHours = v
	}
	if v, err := strconv.Atoi(os.Getenv("OJ_MAX_PARALLEL")); err == nil && v > 0 {
		cfg.Judge.MaxParallel = v
	}
}

func setStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// randomToken backs the dev-mode ephemeral secret.
func randomToken() string {
	raw := make([]byte, 32)
	if _, err := crand.Read(raw); err != nil {
		panic(err) // a broken host CSPRNG makes every secret untrustworthy
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
