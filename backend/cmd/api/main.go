// Command api is the OJ web API server: REST + WebSocket + the gRPC
// JudgeRelay that judge daemons connect to.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/handler"
	"github.com/ysnb/oj/internal/judgehub"
	"github.com/ysnb/oj/internal/logx"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/queue"
	"github.com/ysnb/oj/internal/store"
	"github.com/ysnb/oj/internal/wsq"
	pb "github.com/ysnb/oj/pb"
	"github.com/ysnb/oj/pkg/judge"
)

func main() {
	// --config / OJ_CONFIG are operator inputs (the principal starting the
	// service); config.Load constrains them to .yaml files without traversal
	// segments so a wrong env var cannot point the process at arbitrary
	// non-config files (security-audit §13.2/§13.5).
	cfgPath := os.Getenv("OJ_CONFIG")
	if len(os.Args) > 1 && os.Args[1] == "--config" && len(os.Args) > 2 {
		cfgPath = os.Args[2]
	}
	if err := run(cfgPath); err != nil {
		// structured logger may not exist yet if config loading failed;
		// logx.Init in run covers the rest of the lifetime.
		slog.Error("api", "err", err)
		os.Exit(1)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	logx.Init(logx.ParseLevel(cfg.LogLevel))
	slog.Info("api starting", "mode", cfg.Mode, "listen", cfg.Listen, "log_level", cfg.LogLevel)
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if cfg.JWT.Secret == "" && cfg.Mode == "dev" {
		// Dev-only convenience: ephemeral secret, never a literal in source.
		cfg.JWT.Secret = randomSecret()
		slog.Warn("dev mode: generated ephemeral OJ_JWT_SECRET")
	}
	if cfg.JWT.DaemonSecret == "" {
		cfg.JWT.DaemonSecret = randomSecret()
		slog.Warn("OJ_DAEMON_SECRET not set; generated one for this run")
	}

	db, err := store.Open(cfg)
	if err != nil {
		return err
	}
	if err := bootstrapAdmin(db); err != nil {
		return err
	}
	q, err := queue.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		return err
	}
	defer q.Close()

	jwtMgr := auth.NewManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)
	wsHub := wsq.NewHub()
	langs, err := judge.NewRegistry("")
	if err != nil {
		return err
	}
	hub := judgehub.New(db, q, wsHub, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hub.StartRequeueScanner(ctx)
	// RequeuePending(ctx) — restore PENDING tasks lost from the queue by a
	// restart; must run before daemons start pulling.
	hub.RequeuePending(ctx)

	grpcSrv := grpc.NewServer()
	pb.RegisterJudgeRelayServer(grpcSrv, hub)
	go func() {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			slog.Error("grpc listen", "err", err)
			stop()
			return
		}
		slog.Info("grpc listening", "addr", cfg.GRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			slog.Error("grpc serve", "err", err)
		}
	}()

	server := &handler.Server{
		DB: db, Cfg: cfg, JWT: jwtMgr, Queue: q, WS: wsHub, Langs: langs, JudgeHub: hub,
	}
	// External practice sync (刷题统计报表): hourly crawl of bound users'
	// external-platform submission logs via the plugin registry.
	server.StartExternalSyncScanner(ctx)
	httpSrv := &http.Server{
		Addr:    cfg.Listen,
		Handler: server.Router(),
		// Slow-loris and stuck-connection guards; uploads are small zips so
		// a 60s read budget is ample.
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		slog.Info("http listening", "addr", cfg.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http serve", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	grpcSrv.GracefulStop()
	slog.Info("api stopped")
	return nil
}

// bootstrapAdmin creates the initial admin from OJ_ADMIN_USERNAME /
// OJ_ADMIN_PASSWORD on first boot only; no default credentials exist.
// The initial admin is THE one super_admin with id 0 (root convention):
// fresh installs get uid 0; role uniqueness (exactly one super_admin) is
// enforced at grant time with automatic succession.
func bootstrapAdmin(db *gorm.DB) error {
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		return nil
	}
	username, password := os.Getenv("OJ_ADMIN_USERNAME"), os.Getenv("OJ_ADMIN_PASSWORD")
	if username == "" || password == "" {
		slog.Warn("no users yet; set OJ_ADMIN_USERNAME/OJ_ADMIN_PASSWORD to create the first admin")
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	// map-based create so id=0 survives GORM's zero-value omission (a
	// struct create would drop ID 0 and let the sequence assign 1)
	if err := db.Model(&model.User{}).Create(map[string]any{
		"id":            0,
		"username":      username,
		"password_hash": hash,
		"nickname":      "Admin",
		"role":          model.RoleSuperAdmin,
	}).Error; err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	slog.Info("bootstrap admin created", "username", username, "uid", 0, "role", "super_admin")
	return nil
}

func randomSecret() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}
