#!/usr/bin/env bash
# YSNB OJ — one-shot deployment (Docker Compose route).
# ... (unchanged header)
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== 0. preconditions ="
command -v docker >/dev/null || {
  echo "docker missing — install it from your distro packages first"
  echo "(steps in docs/deploy.md, 方式 A 前置)"
  exit 1
}
[ -f frontend/dist/index.html ] || {
  echo "frontend/dist missing — on a dev box run: cd frontend && npm ci && npm run build, then sync dist here"
  exit 1
}
[ "$(stat -fc %T /sys/fs/cgroup)" = "cgroup2fs" ] || {
  echo "cgroup v2 not enabled (stat -fc %T /sys/fs/cgroup should print cgroup2fs)"
  exit 1
}

echo "== 1. generate .env (skipped when present) ="
if [ ! -f .env ]; then
  # All secret values are generated in-process by openssl — nothing hardcoded.
  # Distinct placeholder keys per secret line (two identical placeholders
  # would collapse to one value under a chained sed).
  PG_PW=$(openssl rand -hex 16)
  JWT=$(openssl rand -base64 32 | tr -d '\n')
  DAEMON=$(openssl rand -base64 32 | tr -d '\n')
  ADMIN_PW=$(openssl rand -hex 10)
  CLI_TOKEN=$(openssl rand -hex 16)
  sed -e "s|change-me-postgres|$PG_PW|" \
      -e "s|^OJ_JWT_SECRET=.*|OJ_JWT_SECRET=$JWT|" \
      -e "s|^OJ_DAEMON_SECRET=.*|OJ_DAEMON_SECRET=$DAEMON|" \
      -e "s|change-me-admin|$ADMIN_PW|" \
      -e "s|change-me-cli-token|$CLI_TOKEN|" \
      .env.example > .env
  chmod 600 .env
  echo ".env generated with random secrets. Admin password (record it now):"
  grep '^OJ_ADMIN_PASSWORD=' .env
else
  echo ".env already exists — keeping it"
fi

echo "== 2. build & start ="
docker compose up -d --build

echo "== 3. wait for API ="
code=""
for _ in $(seq 1 30); do
  code=$(curl -s -o /dev/null -m 3 -w "%{http_code}" http://127.0.0.1:8080/api/v1/languages || true)
  [ "$code" = "200" ] && break
  sleep 2
done
[ "$code" = "200" ] || { echo "API not ready — logs: docker compose logs api"; exit 1; }
echo "API OK"

echo "== 4. judge selftest ="
docker compose exec -T judge oj-judge --selftest || {
  echo "selftest failed — logs: docker compose logs judge"; exit 1; }

echo "== 5. backup cron ="
mkdir -p /opt/oj/backup
install -m 755 deploy/backup.sh /opt/oj/backup.sh
install -m 755 deploy/restore-drill.sh /opt/oj/restore-drill.sh
( crontab -l 2>/dev/null | grep -v 'backup.sh\|restore-drill.sh' || true;
  echo '0 3 * * * /opt/oj/backup.sh >> /opt/oj/backup/backup.log 2>&1';
  echo '0 4 1 * * /opt/oj/restore-drill.sh $(ls -t /opt/oj/backup/db/*.dump 2>/dev/null | head -1) >> /opt/oj/backup/drill.log 2>&1' ) | crontab -
echo "cron installed"

echo "== 6. smoke ="
code=$(curl -s -o /dev/null -m 5 -w "%{http_code}" http://127.0.0.1/ || true)
[ "$code" = "200" ] && echo "frontend OK" || echo "frontend abnormal (http $code) — check web container / frontend/dist"

echo
echo "===== deployment complete ====="
echo "URL: http://<server-ip>/"
echo "admin user: $(grep '^OJ_ADMIN_USERNAME=' .env | cut -d= -f2)"
echo "admin pass: $(grep '^OJ_ADMIN_PASSWORD=' .env | cut -d= -f2)   <- record it now"
echo "first invite code: log into admin console -> 用户管理 -> 邀请码 -> 生成"
echo
echo "maintenance CLI (create/recover super admin, doctor…):"
echo "  docker compose exec api oj-cli --token \$(grep '^OJ_CLI_TOKEN=' .env | cut -d= -f2) <子命令>"
echo "  子命令: create-superadmin / reset-password / set-role / users / invite / doctor"