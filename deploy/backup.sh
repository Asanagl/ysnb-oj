#!/usr/bin/env bash
# YSNB OJ — full backup: PostgreSQL + data dir (testdata/uploads).
# Install on the API machine as /opt/oj/backup.sh, then:
#   chmod +x /opt/oj/backup.sh
#   crontab -e:  0 3 * * * /opt/oj/backup.sh >> /opt/oj/backup/backup.log 2>&1
# Retention: keeps the last $KEEP_DAYS daily dumps (plus 1st-of-month dumps
# for $KEEP_MONTHS months), so a silent corruption has a longer window to fix.
set -u

KEEP_DAYS=${KEEP_DAYS:-14}
KEEP_MONTHS=${KEEP_MONTHS:-6}
BACKUP_ROOT=${BACKUP_ROOT:-/opt/oj/backup}
DATA_DIR=${DATA_DIR:-/opt/oj/data}
# Docker Compose deployments talk to the "postgres" container; bare-metal
# (systemd) uses peer auth as the postgres OS user via runuser.
PG_CONTAINER=${PG_CONTAINER:-$(docker ps --format "{{.Names}}" 2>/dev/null | grep -i postgres | head -1)}
PG_USER=${PG_USER:-oj}
PG_DB=${PG_DB:-oj}

TS=$(date +%F)
STAMP=$(date '+%F %T')
mkdir -p "$BACKUP_ROOT/db" "$BACKUP_ROOT/data"

log() { echo "[$STAMP] $*"; }

# 1. PostgreSQL dump (custom format -> pg_restore capable, compressed)
DB_OUT="$BACKUP_ROOT/db/oj-$TS.dump"
if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$PG_CONTAINER"; then
  docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -Fc "$PG_DB" > "$DB_OUT"
elif command -v runuser >/dev/null 2>&1; then
  runuser -u postgres -- pg_dump -Fc "$PG_DB" > "$DB_OUT" 2>/dev/null
else
  su - postgres -c "pg_dump -Fc $PG_DB" > "$DB_OUT" 2>/dev/null
fi
if [ ! -s "$DB_OUT" ]; then
  log "ERROR: db dump empty: $DB_OUT"
  exit 1
fi
log "db dump ok: $DB_OUT ($(du -h "$DB_OUT" | cut -f1))"

# 2. data dir snapshot (testdata, uploads, codes). rsync keeps it incremental;
# hosts without rsync fall back to a full cp.
SNAP="$BACKUP_ROOT/data/snapshot"
if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete "$DATA_DIR/" "$SNAP/"
else
  rm -rf "$SNAP"
  mkdir -p "$(dirname "$SNAP")"
  cp -a "$DATA_DIR" "$SNAP"
fi
log "data snapshot ok: $SNAP ($(du -sh "$SNAP" | cut -f1))"

# 3. retention: prune daily dumps beyond KEEP_DAYS (keep 1st-of-month dumps)
find "$BACKUP_ROOT/db" -name 'oj-*.dump' -mtime +"$KEEP_DAYS" ! -name 'oj-01.dump' -delete 2>/dev/null || true
find "$BACKUP_ROOT/db" -type f -name 'oj-*-01.dump' -mtime +$((KEEP_MONTHS * 30)) -delete 2>/dev/null || true
log "retention applied (daily>$KEEP_DAYS, monthly>$KEEP_MONTHS months)"

log "backup finished"
