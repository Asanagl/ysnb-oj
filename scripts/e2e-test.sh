#!/usr/bin/env bash
# e2e-test.sh — self-contained API E2E suite for YSNB OJ.
# Builds the oj-api binary, boots it against a throwaway sqlite data dir,
# walks the normal + adversarial flows with per-step PASS/FAIL, then tears
# everything down. Exit code 0 = all checks passed. Safe to re-run at any
# time. Needs: go, node, curl on PATH (any OS with a POSIX shell).
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 1

BASE=127.0.0.1:18081
API="http://$BASE/api/v1"
DAEMON_SECRET="E2eDaemonSecret-2026"
ADMIN_USER=e2e-admin
ADMIN_PASS="E2eLocalOnly-2026"
USER_PASS="E2eUserPass-2026"
export BODY="$ROOT/.e2e-body.tmp"

PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "PASS: $1"; }
bad() { FAIL=$((FAIL+1)); echo "FAIL: $1"; }
# check <name> <expected> <actual>
check() {
  if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$2] got [$3])"; fi
}
# status <name> <expected-code> <curl args...> — runs curl, saves body, checks code
status() {
  local name=$1 want=$2; shift 2
  local got; got=$(curl -s -o "$BODY" -w "%{http_code}" "$@")
  check "$name" "$want" "$got"
}
# jget <dotted.path> — print a field from the last response body
jget() {
  node -e '
    const fs=require("fs");
    let d; try{ d=JSON.parse(fs.readFileSync(process.env.BODY,"utf8")); }catch(e){ console.log(""); process.exit(0); }
    let cur=d;
    for(const p of process.argv[1].split(".")){
      if(cur==null) break;
      cur=Array.isArray(cur)?cur[Number(p)]:cur[p];
    }
    console.log(cur===undefined||cur===null?"":cur);
  ' "$1"
}
# jtest <js expression over parsed body `d`> — prints 1/0
jtest() {
  node -e '
    const fs=require("fs");
    let d; try{ d=JSON.parse(fs.readFileSync(process.env.BODY,"utf8")); }catch(e){ console.log(0); process.exit(0); }
    let r=false; try{ r=!!eval(process.argv[1]); }catch(e){}
    console.log(r?1:0);
  ' "$1"
}
# jcheck <name> <js expr> — PASS when expr is truthy over last body
jcheck() {
  if [ "$(jtest "$2")" = "1" ]; then ok "$1"; else bad "$1 (expr: $2)"; fi
}
postj() { # postj <url> <json-string> [extra curl args...]
  curl -s -o "$BODY" -w "%{http_code}" -X POST -H "Content-Type: application/json" -d "$2" "${@:3}" "$1"
}

cleanup() {
  if [ -f "$ROOT/.e2e-api.pid" ]; then
    E2E_PID=$(cat "$ROOT/.e2e-api.pid")
    kill "$E2E_PID" >/dev/null 2>&1
    # wait for the process to actually exit before rm — otherwise the db
    # file is still held and cleanup fails with "Device or resource busy"
    for _ in $(seq 1 20); do
      kill -0 "$E2E_PID" 2>/dev/null || break
      sleep 0.2
    done
  fi
  rm -rf "$ROOT/data-e2e" "$ROOT/data-e2e.log" "$ROOT/.e2e-body.tmp" \
         "$ROOT/.e2e-stage" "$ROOT/.e2e-testdata.zip" "$ROOT/.e2e-garbage.txt" \
         "$ROOT/.e2e-traversal.zip" "$ROOT/.e2e-big.json" "$ROOT/.e2e-bigcode.json" \
         "$ROOT/.e2e-users.csv" "$ROOT/.e2e-conc."* "$ROOT/.e2e-api.pid" "$ROOT/.e2e-api-bin"
}
trap cleanup EXIT

echo "=== setup ==="
rm -rf "$ROOT/data-e2e" "$ROOT/data-e2e.log"
go -C backend build -o "$ROOT/.e2e-api-bin" ./cmd/api || { echo "FATAL: build failed"; exit 1; }
OJ_MODE=dev OJ_DATA_DIR=./data-e2e OJ_DB_DRIVER=sqlite OJ_DB_DSN=./data-e2e/oj.db \
OJ_LISTEN=$BASE OJ_GRPC_ADDR=127.0.0.1:19091 \
OJ_ADMIN_USERNAME=$ADMIN_USER OJ_ADMIN_PASSWORD=$ADMIN_PASS \
OJ_JWT_SECRET=E2eJwtSecret-LocalOnly-2026 OJ_DAEMON_SECRET=$DAEMON_SECRET \
"$ROOT/.e2e-api-bin" > data-e2e.log 2>&1 &
echo $! > "$ROOT/.e2e-api.pid"
for i in $(seq 1 40); do
  curl -s -o /dev/null "$API/languages" && break
  sleep 0.5
done
READY=$(curl -s -o /dev/null -w "%{http_code}" "$API/languages")
[ "$READY" = "200" ] || { echo "FATAL: server did not come up (last code $READY); tail of data-e2e.log:"; tail -20 data-e2e.log; exit 1; }

echo "=== 1. public endpoints ==="
status "GET /languages is 200" 200 "$API/languages"
jcheck "languages include cpp" 'd.some(x=>x.id==="cpp")'
status "GET /auth/me without token is 401" 401 "$API/auth/me"

echo "=== 2. admin login ==="
CODE=$(postj "$API/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
check "admin login is 200" 200 "$CODE"
ADMIN_TOKEN=$(jget token)
jcheck "admin role is admin" 'd.user.role==="admin"'
status "admin login wrong password is 401" 401 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"wrong-pass\"}" "$API/auth/login"
status "GET /auth/me with admin token is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/auth/me"

echo "=== 3. invite + register ==="
postj "$API/admin/invite-codes" '{"max_uses":1}' -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
# reuse the saved body from the POST above
INVITE1=$(jget code)
[ -n "$INVITE1" ] && ok "invite code created" || bad "invite code empty"
status "register with invite code is 200" 200 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"e2e-user\",\"password\":\"$USER_PASS\",\"nickname\":\"E2E User\",\"student_no\":\"2026E2E0001\",\"invite_code\":\"$INVITE1\"}" "$API/auth/register"
status "duplicate username register is 400" 400 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"e2e-user\",\"password\":\"$USER_PASS\",\"student_no\":\"2026E2E0001\",\"invite_code\":\"$INVITE1\"}" "$API/auth/register"
status "invalid invite code register is 400" 400 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"e2e-other\",\"password\":\"$USER_PASS\",\"invite_code\":\"NOPE1234\"}" "$API/auth/register"
status "exhausted invite code register is 400" 400 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"e2e-another\",\"password\":\"$USER_PASS\",\"invite_code\":\"$INVITE1\"}" "$API/auth/register"

echo "=== 4. RBAC ==="
CODE=$(postj "$API/auth/login" "{\"username\":\"e2e-user\",\"password\":\"$USER_PASS\"}")
check "e2e-user login is 200" 200 "$CODE"
USER_TOKEN=$(jget token); USER_ID=$(jget user.id)
status "user GET /admin/users is 403" 403 -H "Authorization: Bearer $USER_TOKEN" "$API/admin/users"
status "user GET /internal/testdata without daemon token is 401" 401 -H "Authorization: Bearer $USER_TOKEN" "$API/internal/testdata/1/1/in"
status "GET /internal/testdata with wrong daemon token is 401" 401 -H "Authorization: Bearer wrong-daemon" "$API/internal/testdata/1/1/in"

echo "=== 5. CSV import ==="
printf 'csvuser1,2026001,Csv One,\n,,\ncsvuser2,2026002,Csv Two,\n' > "$ROOT/.e2e-users.csv"
status "CSV import is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@$ROOT/.e2e-users.csv" "$API/admin/users/import"
jcheck "CSV created == 2 (1 bad row skipped)" 'd.created===2'
jcheck "CSV bad row has error" 'typeof d.rows[1].err==="string" && d.rows[1].err.length>0'
CSV_PASS=$(jget rows.0.password)
[ -n "$CSV_PASS" ] && ok "CSV generated password returned" || bad "CSV generated password missing"
status "imported user can log in with generated password" 200 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"csvuser1\",\"password\":\"$CSV_PASS\"}" "$API/auth/login"
USER2_TOKEN=$(jget token)

echo "=== 6. problem CRUD ==="
PROB_JSON='{"title":"E2E Sum Problem","statement_md":"# Sum\nCompute a+b.","input_desc":"two ints","output_desc":"sum","hint":"","source":"e2e","tags":["math","beginner"],"samples":[{"input":"1 2","output":"3"},{"input":"5 7","output":"12"}],"time_limit_ms":1000,"mem_limit_mb":256,"visibility":"members","judge_mode":"default"}'
postj "$API/problems" "$PROB_JSON" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
PROB_ID=$(jget id)
[ -n "$PROB_ID" ] && ok "problem created id=$PROB_ID" || bad "problem create failed"
status "problem list is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/problems"
jcheck "list contains created problem" 'd.items.some(p=>p.id==='"$PROB_ID"')'
status "admin GET problem detail is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/problems/$PROB_ID"
jcheck "detail has 2 samples" 'd.samples.length===2'
jcheck "admin can manage" 'd.can_manage===true'
status "user GET members problem is 200" 200 -H "Authorization: Bearer $USER_TOKEN" "$API/problems/$PROB_ID"
jcheck "user cannot manage" 'd.can_manage===false'
postj "$API/problems" '{"title":"E2E Hidden Problem","visibility":"hidden"}' -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
HIDDEN_ID=$(jget id)
status "user list does not contain hidden problem" 200 -H "Authorization: Bearer $USER_TOKEN" "$API/problems?q=E2E%20Hidden"
jcheck "hidden absent from user list" 'd.items.every(p=>p.id!=='"$HIDDEN_ID"')'
status "user GET hidden problem is 404" 404 -H "Authorization: Bearer $USER_TOKEN" "$API/problems/$HIDDEN_ID"
status "admin GET hidden problem is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/problems/$HIDDEN_ID"
status "user PUT problem is 403" 403 -X PUT -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"hijack"}' "$API/problems/$PROB_ID"
status "user POST /problems is 403" 403 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"nope"}' "$API/problems"
status "PUT problem with empty title is 400" 400 -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"   "}' "$API/problems/$PROB_ID"
status "PUT problem with non-array tags is 400" 400 -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"E2E Sum Problem","tags":"not-an-array"}' "$API/problems/$PROB_ID"
status "PUT problem valid rename is 200" 200 -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"E2E Sum Problem v2","visibility":"members","judge_mode":"default"}' "$API/problems/$PROB_ID"

echo "=== 7. testdata upload ==="
mkdir -p "$ROOT/.e2e-stage"
printf '1 2\n'    > "$ROOT/.e2e-stage/1.in";  printf '3\n'  > "$ROOT/.e2e-stage/1.out"
printf '10 20\n' > "$ROOT/.e2e-stage/2.in";  printf '30\n' > "$ROOT/.e2e-stage/2.out"
go run scripts/zipmake.go -mode testdata -out "$ROOT/.e2e-testdata.zip" || bad "zipmake testdata failed"
status "testdata zip upload is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@$ROOT/.e2e-testdata.zip" "$API/problems/$PROB_ID/testdata"
jcheck "stored 2 cases" 'd.stored===2'
status "GET testdata list is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/problems/$PROB_ID/testdata"
jcheck "testdata has 2 entries with sha256" 'd.length===2 && d.every(c=>/^[0-9a-f]{64}$/.test(c.input_sha))'
printf 'this is not a zip' > "$ROOT/.e2e-garbage.txt"
status "non-zip upload is 400" 400 -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@$ROOT/.e2e-garbage.txt" "$API/problems/$PROB_ID/testdata"
go run scripts/zipmake.go -mode traversal -out "$ROOT/.e2e-traversal.zip" || bad "zipmake traversal failed"
status "traversal zip upload is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@$ROOT/.e2e-traversal.zip" "$API/problems/$PROB_ID/testdata"
EVIL=0
for f in "$ROOT/evil.txt" "$ROOT/data-e2e/evil.txt" "$ROOT/backend/evil.txt" "$ROOT/data-e2e/testdata/evil.txt" "$ROOT/data-e2e/testdata/$PROB_ID/evil.txt"; do
  [ -e "$f" ] && EVIL=1
done
check "no evil.txt escaped the data dir" 0 "$EVIL"
status "daemon token serves testdata blob" 200 -H "Authorization: Bearer $DAEMON_SECRET" "$API/internal/testdata/$PROB_ID/1/in"
check "served blob content matches" "5 5" "$(tr -d '\n' < "$BODY")"

echo "=== 8. submission flow ==="
S1=$(postj "$API/submissions" "{\"problem_id\":$PROB_ID,\"language\":\"cpp\",\"code\":\"int main(){return 0;}\"}" -H "Authorization: Bearer $USER_TOKEN")
check "valid submit is 200" 200 "$S1"
SUB1_ID=$(jget id)
jcheck "new submission is PENDING" 'd.status==="PENDING"'
status "unknown language submit is 400" 400 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_id\":$PROB_ID,\"language\":\"cobol\",\"code\":\"x\"}" "$API/submissions"
status "empty code submit is 400" 400 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_id\":$PROB_ID,\"language\":\"cpp\",\"code\":\"\"}" "$API/submissions"
node -e 'require("fs").writeFileSync(".e2e-bigcode.json",JSON.stringify({problem_id:'"$PROB_ID"',language:"cpp",code:"A".repeat(300*1024)}))'
status "oversize code (>256KB) submit is 400" 400 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d @"$ROOT/.e2e-bigcode.json" "$API/submissions"
status "hidden problem submit is 404" 404 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_id\":$HIDDEN_ID,\"language\":\"cpp\",\"code\":\"x\"}" "$API/submissions"

echo "=== 9. contests ==="
NOW=$(date -u +%s)
START1=$(date -u -d @$((NOW-3600)) +%Y-%m-%dT%H:%M:%SZ); END1=$(date -u -d @$((NOW+7200)) +%Y-%m-%dT%H:%M:%SZ)
START2=$(date -u -d @$((NOW+3600)) +%Y-%m-%dT%H:%M:%SZ); END2=$(date -u -d @$((NOW+10800)) +%Y-%m-%dT%H:%M:%SZ)
postj "$API/contests" "{\"title\":\"E2E Contest\",\"mode\":\"acm\",\"visibility\":\"public\",\"start_time\":\"$START1\",\"end_time\":\"$END1\"}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
CONTEST_ID=$(jget id)
[ -n "$CONTEST_ID" ] && ok "contest created id=$CONTEST_ID" || bad "contest create failed"
# 新建比赛默认报名制（M3 起）：不报名的提交会被 400 拒绝
status "contest registration is 200" 200 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d '{"team_name":"E2E Solo","team_type":"official"}' "$API/contests/$CONTEST_ID/register"
status "attach problem to contest is 200" 200 -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_ids\":[$PROB_ID]}" "$API/contests/$CONTEST_ID/problems"
# 挂题生成独立副本（M3 起）：比赛提交必须打副本 id，原题 id 不属于比赛
curl -s -o "$BODY" -H "Authorization: Bearer $USER_TOKEN" "$API/contests/$CONTEST_ID"
CONTEST_PROB_ID=$(jget problems.0.id)
[ -n "$CONTEST_PROB_ID" ] && ok "contest problem copy id=$CONTEST_PROB_ID" || bad "no contest problem copy"
postj "$API/contests" "{\"title\":\"E2E Future Contest\",\"visibility\":\"public\",\"start_time\":\"$START2\",\"end_time\":\"$END2\"}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
FUTURE_ID=$(jget id)
postj "$API/problems" '{"title":"E2E Unlinked Problem","visibility":"members"}' -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
UNLINKED_ID=$(jget id)
S2=$(postj "$API/submissions" "{\"problem_id\":$CONTEST_PROB_ID,\"contest_id\":$CONTEST_ID,\"language\":\"cpp\",\"code\":\"int main(){return 0;}\"}" -H "Authorization: Bearer $USER_TOKEN")
check "in-window contest submit is 200" 200 "$S2"
SUB2_ID=$(jget id)
status "future-window contest submit is 400" 400 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_id\":$PROB_ID,\"contest_id\":$FUTURE_ID,\"language\":\"cpp\",\"code\":\"x\"}" "$API/submissions"
status "unlinked problem with contest_id is 400" 400 -X POST -H "Authorization: Bearer $USER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"problem_id\":$UNLINKED_ID,\"contest_id\":$CONTEST_ID,\"language\":\"cpp\",\"code\":\"x\"}" "$API/submissions"
status "standings is 200" 200 -H "Authorization: Bearer $USER_TOKEN" "$API/contests/$CONTEST_ID/standings"
jcheck "standings row for submitter exists" 'd.rows.some(r=>r.user_id==='"$USER_ID"')'
jcheck "PENDING submissions solve nothing" 'd.rows.every(r=>r.solved===0 && r.penalty_ms===0)'

echo "=== 10. submission rate limit (15/min) ==="
LIMIT_SAW_429=0; EARLY_429=0
for i in $(seq 1 10); do
  # requests 9..18 for this user; the limiter allows exactly 15 per window
  c=$(postj "$API/submissions" "{\"problem_id\":$PROB_ID,\"language\":\"cpp\",\"code\":\"loop $i\"}" -H "Authorization: Bearer $USER_TOKEN")
  [ "$c" = "429" ] && LIMIT_SAW_429=1
  if [ "$i" -le 7 ] && [ "$c" = "429" ]; then EARLY_429=1; fi
done
check "429 appeared once over 15/min" 1 "$LIMIT_SAW_429"
check "no 429 before the 16th request" 0 "$EARLY_429"

echo "=== 11. rejudge + daemons ==="
status "admin rejudge existing submission is 200" 200 -X POST -H "Authorization: Bearer $ADMIN_TOKEN" "$API/admin/rejudge/$SUB1_ID"
status "admin rejudge missing submission is 404" 404 -X POST -H "Authorization: Bearer $ADMIN_TOKEN" "$API/admin/rejudge/999999"
status "admin GET /admin/daemons is 200" 200 -H "Authorization: Bearer $ADMIN_TOKEN" "$API/admin/daemons"
jcheck "daemons response has queue_length" 'typeof d.queue_length==="number"'

echo "=== 12. WebSocket ==="
# verdict by probe OUTPUT, not exit code — node/undici can crash non-zero
# at teardown on Windows (libuv assert) even after printing WS-PASS
WS_OUT=$(node scripts/ws-probe.mjs valid "$ADMIN_TOKEN" 2>&1); echo "$WS_OUT"
echo "$WS_OUT" | grep -q "WS-PASS" && ok "WS with valid token" || bad "WS with valid token: $WS_OUT"
WS_OUT=$(node scripts/ws-probe.mjs anon 2>&1); echo "$WS_OUT"
echo "$WS_OUT" | grep -q "WS-PASS" && ok "WS without token rejected" || bad "WS without token rejected: $WS_OUT"

echo "=== 13. security extras ==="
TAMPERED=$(node -e 'const t=process.argv[1];console.log(t.slice(0,30)+(t[30]==="0"?"1":"0")+t.slice(31));' "$USER_TOKEN")
status "tampered token is 401" 401 -H "Authorization: Bearer $TAMPERED" "$API/auth/me"
status "non-Bearer auth scheme is 401" 401 -H "Authorization: Basic $USER_TOKEN" "$API/auth/me"
status "other user can read submission view" 200 -H "Authorization: Bearer $USER2_TOKEN" "$API/submissions/$SUB2_ID"
jcheck "contest code hidden from other user while running" 'd.code===undefined && d.user_id_masked===true'
status "other user can read training submission view" 200 -H "Authorization: Bearer $USER2_TOKEN" "$API/submissions/$SUB1_ID"
jcheck "training code hidden from other user" 'd.code===undefined'
status "owner sees own code" 200 -H "Authorization: Bearer $USER_TOKEN" "$API/submissions/$SUB1_ID"
jcheck "own submission includes code" 'd.code==="int main(){return 0;}"'
node -e 'require("fs").writeFileSync(".e2e-big.json",JSON.stringify({username:"x",password:"y",pad:"A".repeat(1200000)}))'
BIG=$(curl -s -o "$BODY" -w "%{http_code}" -X POST -H "Content-Type: application/json" -d @"$ROOT/.e2e-big.json" "$API/auth/login")
if [ "$BIG" = "400" ] || [ "$BIG" = "413" ]; then ok "oversized login body rejected ($BIG)"; else bad "oversized login body (want 400/413 got $BIG)"; fi

echo "=== 14. concurrent submissions ==="
CONC_PIDS=""
for i in 1 2 3 4 5; do
  curl -s -o "$ROOT/.e2e-conc.$i" -w "%{http_code}" -X POST -H "Authorization: Bearer $USER2_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"problem_id\":$PROB_ID,\"language\":\"cpp\",\"code\":\"concurrent $i\"}" "$API/submissions" > "$ROOT/.e2e-conc.$i.code" &
  CONC_PIDS="$CONC_PIDS $!"
done
# wait only for the curl jobs — never the server background task
wait $CONC_PIDS
CONC_OK=1
for i in 1 2 3 4 5; do
  [ "$(cat "$ROOT/.e2e-conc.$i.code")" = "200" ] || CONC_OK=0
done
check "5 concurrent submissions all 200" 1 "$CONC_OK"
IDS=$(node -e '
  const fs=require("fs");
  const ids=[];
  for(let i=1;i<=5;i++){
    try{ ids.push(JSON.parse(fs.readFileSync(".e2e-conc."+i,"utf8")).id); }catch(e){ ids.push(0); }
  }
  const s=[...ids].sort((a,b)=>a-b);
  const distinct=new Set(ids).size===5;
  const consecutive=s[4]-s[0]===4 && s[0]>0;
  console.log(distinct&&consecutive?"1":"0");
')
check "concurrent ids distinct and consecutive" 1 "$IDS"

echo "=== summary ==="
echo "PASS=$PASS FAIL=$FAIL"
if [ "$FAIL" -eq 0 ]; then echo "E2E: ALL GREEN"; exit 0; else echo "E2E: FAILURES PRESENT"; exit 1; fi
