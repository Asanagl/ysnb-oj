// M5 E2E: plugin system, IOI contest mode, external practice sync API,
// shared export fairness line, case-score management. Targets production.
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const env = Object.fromEntries(
  fs.readFileSync(path.join(here, '.ojenv'), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)
const BASE = env.BASE || 'http://localhost:8080/api/v1'
let passed = 0, failed = 0
function check(name, expected, actual) {
  if (expected === actual) { passed++; console.log(`PASS ${name}: ${actual}`) }
  else { failed++; console.log(`FAIL ${name}: expected ${JSON.stringify(expected)} got ${JSON.stringify(actual)}`) }
}
async function api(method, p, opts = {}) {
  const headers = {}
  if (opts.token) headers.Authorization = `Bearer ${opts.token}`
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  const r = await fetch(BASE + p, { method, headers, body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined })
  const raw = await r.text()
  let data = null
  try { data = JSON.parse(raw) } catch { data = { raw: raw.slice(0, 200) } }
  return { status: r.status, data }
}

const admin = (await api('POST', '/auth/login', { body: { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD } })).data.token
check('admin login', 'string', typeof admin)

// ---- 1. plugin inventory ----
{
  const r = await api('GET', '/admin/plugins', { token: admin })
  check('plugins list 200', 200, r.status)
  check('webhook hook registered', true, (r.data.event_hooks ?? []).includes('webhook'))
  check('cf submit fetcher', true, (r.data.submit_fetchers ?? []).includes('codeforces'))
  check('luogu submit fetcher', true, (r.data.submit_fetchers ?? []).includes('luogu'))
  check('atcoder submit fetcher', true, (r.data.submit_fetchers ?? []).includes('atcoder'))
  check('nowcoder submit fetcher', true, (r.data.submit_fetchers ?? []).includes('nowcoder'))
  check('cf problem source', true, (r.data.problem_sources ?? []).includes('codeforces'))
}

// ---- 2. external problem import (codeforces) ----
let importedId = 0
{
  const r = await api('POST', '/external/problems/import', {
    token: admin, body: { source: 'codeforces', external_id: '1A', visibility: 'hidden' },
  })
  check('external import 200', 200, r.status)
  importedId = r.data.problem?.id ?? 0
  check('external import has id', true, importedId > 0)
  check('external source tag', true, String(r.data.problem?.source ?? '').includes('codeforces'))
  const bad = await api('POST', '/external/problems/import', {
    token: admin, body: { source: 'nonexistent', external_id: '1A' },
  })
  check('unknown plugin rejected', 400, bad.status)
}

// ---- 3. case scores (IOI 分值表) ----
{
  const bad = await api('PUT', `/problems/${importedId}/case-scores`, {
    token: admin, body: { scores: { 1: 50 } },
  })
  check('case score on missing case rejected', 400, bad.status)
}

// ---- 4. shared export (fairness line: no checker, sample-only testdata) ----
{
  const r = await fetch(`${BASE}/problems/${importedId}/export?shared=1`, {
    headers: { Authorization: `Bearer ${admin}` },
  })
  check('shared export 200', 200, r.status)
  const text = await r.text()
  check('shared export has statement', true, text.includes('statement.md'))
  check('shared export no checker', false, text.includes('checker.cpp'))
}

// ---- 5. IOI contest lifecycle ----
{
  const start = new Date(Date.now() - 60_000).toISOString()
  const end = new Date(Date.now() + 3600_000).toISOString()
  const c = await api('POST', '/contests', {
    token: admin,
    body: { title: 'IOI 赛制 E2E', start_time: start, end_time: end, mode: 'ioi', visibility: 'public' },
  })
  check('ioi contest created', 200, c.status)
  check('ioi mode persisted', 'ioi', c.data.mode)
  // create a problem with 2 cases, assign scores 30/70
  const p = await api('POST', '/problems', {
    token: admin,
    body: {
      title: 'IOI 部分分 E2E 题', statement_md: '# IOI\n求和', time_limit_ms: 1000,
      mem_limit_mb: 256, visibility: 'hidden', judge_mode: 'default', tags: [], samples: [],
      contest_id: c.data.id,
    },
  })
  check('ioi problem created', 200, p.status)
  const pid = p.data.id
  // upload two cases: case1 input "1 2" ans "3"; case2 input "10 20" ans "30"
  const fd1 = new FormData()
  fd1.append('input', new Blob(['1 2\n']), '1.in')
  fd1.append('output', new Blob(['3\n']), '1.out')
  await fetch(`${BASE}/problems/${pid}/testdata/case`, { method: 'POST', headers: { Authorization: `Bearer ${admin}` }, body: fd1 })
  const fd2 = new FormData()
  fd2.append('input', new Blob(['10 20\n']), '2.in')
  fd2.append('output', new Blob(['30\n']), '2.out')
  await fetch(`${BASE}/problems/${pid}/testdata/case`, { method: 'POST', headers: { Authorization: `Bearer ${admin}` }, body: fd2 })
  // contest problems are attached as independent copies — the submission
  // must target the copy. setContestProblems links contest-exclusive
  // problems as-is (no clone), so cpid === pid is EXPECTED for exclusive
  // problems created directly inside the contest.
  await api('POST', `/contests/${c.data.id}/problems`, { token: admin, body: { problem_ids: [pid] } })
  await api('POST', `/contests/${c.data.id}/register`, { token: admin, body: { team_name: 'e2e', team_type: 'official' } })
  const cd = await api('GET', `/contests/${c.data.id}`, { token: admin })
  const cpid = (cd.data.problems ?? [])[0]?.id
  check('contest copy has id', true, cpid > 0)
  const sc = await api('PUT', `/problems/${cpid}/case-scores`, { token: admin, body: { scores: { '1': 30, '2': 70 } } })
  check('case scores set', 200, sc.status)
  check('case scores total 100', 100, sc.data.total)
  const scList = await api('GET', `/problems/${cpid}/case-scores`, { token: admin })
  check('case scores readback', 30, (scList.data.scores ?? {})['1'])
  // submit via the contest problem page path (contest-exclusive problems
  // reject direct /submissions by design): language "cpp" + contest_id.
  const lang = (await api('GET', '/languages', { token: admin })).data
  const langId = Array.isArray(lang) ? lang[0]?.id ?? lang.languages?.[0]?.id : null
  // submit a partial solution: prints "3" always → case 1 AC (30 分),
  // case 2 WA (0) — partial credit 30, verdict WA.
  const sub = await api('POST', '/submissions', {
    token: admin,
    body: { problem_id: cpid ?? pid, language: langId ?? 'cpp', contest_id: c.data.id, code: '#include <bits/stdc++.h>\nint main(){std::cout<<3<<std::endl;}' },
  })
  check('ioi submission accepted', 200, sub.status)
  if (sub.status !== 200) console.log('  submission error:', JSON.stringify(sub.data))
  // wait for judging
  let final = null
  for (let i = 0; i < 60; i++) {
    await new Promise((res) => setTimeout(res, 2000))
    const r = await api('GET', `/submissions/${sub.data.id}`, { token: admin })
    if (!['PENDING', 'JUDGING', 'COMPILING'].includes(r.data.status)) { final = r.data; break }
  }
  check('ioi submission judged', true, !!final)
  check('ioi partial score 30', 30, final?.score)
  check('ioi verdict WA (not full)', 'WA', final?.status)
  // standings: admin row with total 30, rank 1
  const meId = (await api('GET', '/auth/me', { token: admin })).data.id
  const st = await api('GET', `/contests/${c.data.id}/standings`, { token: admin })
  check('ioi standings row', true, (st.data.rows ?? []).length >= 1)
  const row = (st.data.rows ?? []).find((x) => x.user_id === meId)
  check('ioi standings total score 30', 30, row?.penalty_ms)
  check('ioi cell score 30', 30, row?.cells?.A?.solved_ms)
}

// ---- 6. external practice bind/sync/report ----
// note: stored=0 on re-run is correct — the dedup index keeps old rows;
// the first-ever bind of a handle pulls hundreds of records.
{
  const r = await api('PUT', '/external/bindings', {
    token: admin, body: { platform: 'codeforces', handle: 'tourist' },
  })
  check('cf bind 200', 200, r.status)
  check('cf sync ok (no error)', '', r.data.error ?? '')
  check('cf synced_at set', true, !!r.data.binding?.synced_at)
  const me = (await api('GET', '/auth/me', { token: admin })).data
  const rep = await api('GET', `/external/report/${me.id}`, { token: admin })
  check('report 200', 200, rep.status)
  check('report platform present', true, (rep.data.by_platform ?? []).some((x) => x.platform === 'codeforces'))
  check('report has records', true, (rep.data.by_platform ?? []).some((x) => x.platform === 'codeforces' && x.submissions > 0))
  check('report activity buckets', true, (rep.data.activity ?? []).length === 365)
  const prof = await api('GET', `/users/${me.id}/profile`, { token: admin })
  check('profile platforms present', true, Array.isArray(prof.data.platforms))
  check('profile platform buckets 365', true, (prof.data.platforms ?? []).length === 365)
  // unbind keeps records; rebind dedups
  const unb = await api('DELETE', '/external/bindings/codeforces', { token: admin })
  check('unbind ok', 200, unb.status)
}

// ---- 7. webhook hook management ----
{
  const bad = await api('PUT', '/admin/hooks/x', { token: admin, body: { url: 'http://127.0.0.1/hook' } })
  check('hook SSRF guard', 400, bad.status)
  const bad2 = await api('PUT', '/admin/hooks/x', { token: admin, body: { url: 'ftp://example.com/x' } })
  check('hook scheme guard', 400, bad2.status)
}

console.log(`\n=== M5 E2E: ${passed} passed, ${failed} failed ===`)
process.exit(failed ? 1 : 0)