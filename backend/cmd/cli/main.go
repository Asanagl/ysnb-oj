// oj-cli — server-side maintenance CLI for YSNB OJ.
//
// Runs INSIDE the api/judge container (docker compose exec api oj-cli …) or
// as /opt/oj/oj-cli on systemd hosts. It talks straight to the configured
// database — no API server needs to be up, which is exactly what you want
// when you've locked yourself out.
//
// Environment: the process env must carry the OJ_* production values. On
// systemd hosts the service files use EnvironmentFile=/opt/oj/oj.env, so
// the same file is sourced for the CLI (oj-env.sh wrapper below does it);
// in Docker the compose environment already carries them.
//
// Every mutating command requires OJ_CLI_TOKEN (a shared random value in
// oj.env / compose environment) passed as --token $OJ_CLI_TOKEN: a double-
// confirmation against accidental invocation, without inventing a network
// entry point. Whoever holds the host shell already holds the database;
// this gate is about deliberate use, not secrecy.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	username := fs.String("username", "", "target username")
	password := fs.String("password", "", "new password")
	role := fs.String("role", "", "target role (user|setter|admin|super_admin)")
	q := fs.String("q", "", "username filter for users")
	token := fs.String("token", "", "confirmation; must equal $OJ_CLI_TOKEN")
	maxUses := fs.Int("max-uses", 30, "invite code max uses")
	hours := fs.Int("hours", 168, "invite code validity in hours")
	_ = fs.Parse(os.Args[2:])

	// mutating commands need the confirmation gate; read-only ones don't.
	mutating := cmd != "users" && cmd != "doctor"
	if mutating && !confirmed(*token) {
		os.Exit(1)
	}

	db := mustDB()
	switch cmd {
	case "create-superadmin":
		createSuperadmin(db, *username, *password)
	case "reset-password":
		resetPassword(db, *username, *password)
	case "set-role":
		setRole(db, *username, *role)
	case "users":
		listUsers(db, *q)
	case "invite":
		mintInvite(db, *maxUses, *hours)
	case "doctor":
		doctor(db, cfg)
	default:
		usage()
		os.Exit(2)
	}
}

// confirmed verifies --token equals $OJ_CLI_TOKEN (const-time compare).
func confirmed(presented string) bool {
	want := os.Getenv("OJ_CLI_TOKEN")
	if want == "" {
		fmt.Fprintln(os.Stderr,
			"oj-cli: 错误：未设置 OJ_CLI_TOKEN 环境变量（在 oj.env / compose environment 配一个随机串）")
		return false
	}
	if !equalConstTime(presented, want) || presented == "" {
		fmt.Fprintln(os.Stderr, "oj-cli: 错误：--token 与 OJ_CLI_TOKEN 不匹配")
		return false
	}
	return true
}

func equalConstTime(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

var cfg *config.Config

// cliEnvFile is the ops env file the CLI sources automatically when the
// process environment lacks OJ_DB_DSN — systemd hosts keep production
// values in /opt/oj/oj.env (EnvironmentFile for oj-api/oj-judge), and the
// CLI must see the same world the daemons see.
const (
	cliEnvFile   = "/opt/oj/oj.env"
	cliConfigFile = "/opt/oj/config.yaml"
)

func mustDB() *gorm.DB {
	loadEnvFileIfPresent(cliEnvFile)
	c, err := config.Load(os.Getenv("OJ_CONFIG"))
	if err != nil {
		fatal("加载配置失败: %v", err)
	}
	cfg = c
	db, err := store.Open(c)
	if err != nil {
		fatal("连接数据库失败: %v", err)
	}
	return db
}

// loadEnvFileIfPresent sources KEY=VALUE lines into the process env (only
// when not already set — real environment wins). This is what lets
// `/opt/oj/oj-cli doctor` work from a bare root shell with zero setup.
func loadEnvFileIfPresent(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || os.Getenv(k) != "" {
			continue // never override a real env var
		}
		_ = os.Setenv(k, v)
	}
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "oj-cli: "+format+"\n", a...)
	os.Exit(1)
}

func ensureNonEmpty(name, v string) {
	if strings.TrimSpace(v) == "" {
		fatal("%s 不能为空", name)
	}
}

func findUser(db *gorm.DB, username string) *model.User {
	u := &model.User{}
	if err := db.Where("username = ?", username).First(u).Error; err != nil {
		fatal("用户 %q 不存在", username)
	}
	return u
}

func createSuperadmin(db *gorm.DB, username, password string) {
	ensureNonEmpty("--username", username)
	ensureNonEmpty("password", password)
	if len(password) < 8 {
		fatal("密码至少 8 位")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fatal("hash 失败: %v", err)
	}
	u := &model.User{}
	err = db.Where("username = ?", username).First(u).Error
	if err != nil {
		if err := db.Create(&model.User{
			Username: username, PasswordHash: hash,
			Nickname: "Admin", Role: model.RoleSuperAdmin,
		}).Error; err != nil {
			fatal("创建失败: %v", err)
		}
		fmt.Printf("超级管理员 %q 已创建\n", username)
		return
	}
	u.Role = model.RoleSuperAdmin
	u.PasswordHash = hash
	u.Banned = false
	if err := db.Save(u).Error; err != nil {
		fatal("更新失败: %v", err)
	}
	fmt.Printf("已将现有用户 %q 提升为 super_admin 并重置密码\n", username)
}

func resetPassword(db *gorm.DB, username, password string) {
	ensureNonEmpty("username", username)
	ensureNonEmpty("password", password)
	if len(password) < 8 {
		fatal("密码至少 8 位")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fatal("hash 失败: %v", err)
	}
	u := findUser(db, username)
	if err := db.Model(u).Updates(map[string]any{"password_hash": hash, "banned": false}).Error; err != nil {
		fatal("重置失败: %v", err)
	}
	fmt.Printf("用户 %q 密码已重置\n", username)
}

func setRole(db *gorm.DB, username, role string) {
	ensureNonEmpty("username", username)
	switch role {
	case model.RoleUser, model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin:
	default:
		fatal("非法角色 %q（可选 user|setter|admin|super_admin）", role)
	}
	u := findUser(db, username)
	if err := db.Model(u).Update("role", role).Error; err != nil {
		fatal("更新失败: %v", err)
	}
	fmt.Printf("用户 %q 角色已设为 %s\n", username, role)
}

func listUsers(db *gorm.DB, keyword string) {
	q := db.Select("id, username, nickname, role, banned, last_login_at").Order("id")
	if strings.TrimSpace(keyword) != "" {
		q = q.Where("username LIKE ?", "%"+keyword+"%")
	}
	var rows []model.User
	q.Find(&rows)
	for _, u := range rows {
		last := "从未"
		if u.LastLoginAt != nil {
			last = u.LastLoginAt.Format("2006-01-02 15:04")
		}
		state := ""
		if u.Banned {
			state = " [已封禁]"
		}
		fmt.Printf("%4d  %-20s %-16s %-12s %s%s\n", u.ID, u.Username, u.Nickname, u.Role, last, state)
	}
}

func mintInvite(db *gorm.DB, maxUses, hours int) {
	if maxUses <= 0 {
		maxUses = 10
	}
	if hours <= 0 {
		hours = 168
	}
	code := randCode(8)
	exp := time.Now().Add(time.Duration(hours) * time.Hour)
	if err := db.Create(&model.InvitationCode{
		Code: code, MaxUses: maxUses, ExpiresAt: &exp,
	}).Error; err != nil {
		fatal("生成失败: %v", err)
	}
	fmt.Printf("邀请码 %s（可用 %d 次，%d 小时后过期）\n", code, maxUses, hours)
}

const inviteAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randCode(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = inviteAlphabet[int(buf[i])%len(inviteAlphabet)]
	}
	return string(buf)
}

func doctor(db *gorm.DB, cfg *config.Config) {
	ok := func(name string, err error) {
		if err != nil {
			fmt.Printf("FAIL %-9s %v\n", name, err)
		} else {
			fmt.Printf("PASS %-9s\n", name)
		}
	}
	var count int64
	err := db.Model(&model.User{}).Count(&count).Error
	ok("database", err)
	if err == nil {
		fmt.Printf("     users=%d\n", count)
	}
	if cfg.Redis.Addr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		perr := rdb.Ping(ctx).Err()
		cancel()
		ok("redis", perr)
	} else {
		fmt.Printf("SKIP %-9s 未配置 OJ_REDIS_ADDR（dev 内存队列模式）\n", "redis")
	}
	var daemons []model.JudgeDaemon
	db.Where("status = ?", model.DaemonOnline).Find(&daemons)
	fmt.Printf("     judge daemons online=%d\n", len(daemons))
	for _, d := range daemons {
		last := "从未"
		if d.LastHeartbeat != nil {
			last = d.LastHeartbeat.Format("15:04:05")
		}
		fmt.Printf("     daemon %s cap=%d active=%d last_hb=%s\n", d.Name, d.Capacity, d.ActiveTasks, last)
	}
}

func usage() {
	fmt.Print(`用法: oj-cli <子命令> [参数]

子命令:
  create-superadmin --username U --password P --token T
      创建（或提升+重置）超级管理员
  reset-password --username U --password P --token T
      重置任意用户密码
  set-role --username U --role R --token T
      设置角色（user|setter|admin|super_admin）
  users [--q 关键字]
      列出用户
  invite --max-uses N --hours H --token T
      生成邀请码
  doctor
      诊断：数据库/Redis/判题机连通性

安全: 除 users/doctor 外，--token 必须等于环境变量 OJ_CLI_TOKEN。
容器内执行: docker compose exec api oj-cli --token $OJ_CLI_TOKEN <子命令> …
`)
}