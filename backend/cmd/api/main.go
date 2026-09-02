// Command api is the OJ web API server: REST + WebSocket + the gRPC
// JudgeRelay that judge daemons connect to.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
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
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/queue"
	"github.com/ysnb/oj/internal/store"
	"github.com/ysnb/oj/internal/wsq"
	pb "github.com/ysnb/oj/pb"
	"github.com/ysnb/oj/pkg/judge"
)

func main() {
	cfgPath := os.Getenv("OJ_CONFIG")
	if len(os.Args) > 1 && os.Args[1] == "--config" && len(os.Args) > 2 {
		cfgPath = os.Args[2]
	}
	if err := run(cfgPath); err != nil {
		log.Fatalf("[api] %v", err)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if cfg.JWT.Secret == "" && cfg.Mode == "dev" {
		// Dev-only convenience: ephemeral secret, never a literal in source.
		cfg.JWT.Secret = randomSecret()
		log.Printf("[api] dev mode: generated ephemeral OJ_JWT_SECRET")
	}
	if cfg.JWT.DaemonSecret == "" {
		cfg.JWT.DaemonSecret = randomSecret()
		log.Printf("[api] OJ_DAEMON_SECRET not set; generated one for this run")
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
			log.Printf("[api] grpc listen: %v", err)
			stop()
			return
		}
		log.Printf("[api] grpc listening on %s", cfg.GRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Printf("[api] grpc serve: %v", err)
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
		log.Printf("[api] http listening on %s", cfg.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[api] http: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	grpcSrv.GracefulStop()
	log.Printf("[api] stopped")
	return nil
}

// bootstrapAdmin creates the initial admin from OJ_ADMIN_USERNAME /
// OJ_ADMIN_PASSWORD on first boot only; no default credentials exist.
func bootstrapAdmin(db *gorm.DB) error {
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		return nil
	}
	username, password := os.Getenv("OJ_ADMIN_USERNAME"), os.Getenv("OJ_ADMIN_PASSWORD")
	if username == "" || password == "" {
		log.Printf("[api] no users yet; set OJ_ADMIN_USERNAME/OJ_ADMIN_PASSWORD to create the first admin")
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := db.Create(&model.User{
		Username: username, PasswordHash: hash,
		Nickname: "Admin", Role: model.RoleAdmin,
	}).Error; err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	log.Printf("[api] bootstrap admin %q created", username)
	return nil
}

func randomSecret() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}
