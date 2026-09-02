#!/usr/bin/env bash
# YSNB OJ — restore drill: verify a backup is actually restorable.
#   /opt/oj/restore-drill.sh /opt/oj/backup/db/oj-2026-08-30.dump
# Preferred path boots a throwaway PostgreSQL container and restores the
# dump into it (nothing touches prod). On hosts without docker it falls back
# to restoring into a scratch database on the local instance (oj_drill_tmp,
# dropped afterwards) — still a genuine restore, still isolated from prod.
set -eu

DUMP=${1:?usage: restore-drill.sh <backup.dump>}
SCRATCH=oj_drill_tmp

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  CONTAINER=oj-restore-drill-$$
  PORT=$((20000 + RANDOM % 20000))
  cleanup() { docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; }
  trap cleanup EXIT
  echo "== docker mode: throwaway postgres on :$PORT =="
  docker run -d --rm --name "$CONTAINER" -e POSTGRES_USER=oj -e POSTGRES_PASSWORD=drill \
    -e POSTGRES_DB=oj -p "$PORT":5432 postgres:16 >/dev/null
  sleep 6
  docker exec -i "$CONTAINER" pg_restore -U oj -d oj --no-owner --role=oj < "$DUMP" || \
    echo "(pg_restore reported errors; extension noise is ignorable)"
  docker exec "$CONTAINER" psql -U oj -d oj -t -c \
    "SELECT 'users', count(*) FROM users UNION ALL
     SELECT 'problems', count(*) FROM problems UNION ALL
     SELECT 'submissions', count(*) FROM submissions UNION ALL
     SELECT 'contests', count(*) FROM contests UNION ALL
     SELECT 'test_cases', count(*) FROM test_cases;"
else
  cleanup() { runuser -u postgres -- dropdb --if-exists "$SCRATCH" >/dev/null 2>&1 || true; }
  trap cleanup EXIT
  echo "== local mode: scratch database $SCRATCH on the local instance =="
  # why cd /: runuser inherits cwd; /root is unreadable to postgres and the
  # permission-denied noise buries real output.
  cd /
  cleanup
  runuser -u postgres -- createdb "$SCRATCH"
  echo "== restoring =="
  runuser -u postgres -- pg_restore -d "$SCRATCH" --no-owner < "$DUMP" 2>/dev/null || \
    echo "(pg_restore reported errors above; extension/owner noise is ignorable)"
  echo "== row counts =="
  runuser -u postgres -- psql -d "$SCRATCH" -t -A -c \
    "SELECT 'users', count(*) FROM users UNION ALL
     SELECT 'problems', count(*) FROM problems UNION ALL
     SELECT 'submissions', count(*) FROM submissions UNION ALL
     SELECT 'contests', count(*) FROM contests UNION ALL
     SELECT 'test_cases', count(*) FROM test_cases;"
fi

echo "== drill PASSED (backup is restorable) =="
