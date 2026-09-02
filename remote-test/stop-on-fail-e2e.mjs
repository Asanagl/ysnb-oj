// Stop-on-fail verification: create a live contest, submit a WA solution
// whose problem has 30 cases, assert verdict WA and that cases after the
// first failure are SKIPPED. Then cleanup.
import fs from 'fs'
import path from 'path'
import { execSync } from 'child_process'
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
async function api(method, p, token, body) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined && !(body instanceof FormData)) headers['Content-Type'] = 'application/json'
  const r = await fetch(BASE + p, { method, headers, body: body !== undefined ? (body instanceof FormData ? body : JSON.stringify(body)) : undefined })
  const text = await r.text()
  try { return JSON.parse(text) } catch { return { raw: text.slice(0, 200) } }
}

const admin = (await api('POST', '/auth/login', null,
  { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).token
if (!admin) throw new Error('admin login failed')

// problem with 30 identical cases; WA solution differs on every case
const prob = await api('POST', '/problems', admin, {
  title: 'stop-on-fail ' + Date.now().toString(36).slice(-4),
  statement_md: '<p>sum</p>', time_limit_ms: 1000, mem_limit_mb: 256,
  judge_mode: 'default', samples: [], tags: [],
})
{
  const pyFile = path.join(here, 'tmp-sof.py')
  fs.writeFileSync(pyFile, `import zipfile, sys
z = zipfile.ZipFile(sys.argv[1], 'w')
for i in range(1, 31):
    z.writestr(f'{i}.in', '5\\n')
    z.writestr(f'{i}.out', '5\\n')
z.close()
`)
  execSync(`python "${pyFile}" "${path.join(here, 'sof.zip')}"`)
  const form = new FormData()
  form.append('file', new Blob([fs.readFileSync(path.join(here, 'sof.zip'))]), 'sof.zip')
  const up = await api('POST', `/problems/${prob.id}/testdata`, admin, form)
  check('upload-30', 30, up.stored)
}

// live contest requiring registration; attaching creates an independent
// COPY of the problem (id changes!) — read the label mapping back.
const START = new Date(Date.now() - 60000).toISOString()
const END = new Date(Date.now() + 3600000).toISOString()
const contest = await api('POST', '/contests', admin, {
  title: 'sof-contest', description: 'stop-on-fail verify',
  start_time: START, end_time: END, visibility: 'public',
})
await api('POST', `/contests/${contest.id}/problems`, admin, { problem_ids: [prob.id] })
const cd = await api('GET', `/contests/${contest.id}`, admin)
const copyId = (cd.problems || [])[0]?.id
if (!copyId) throw new Error('no contest copy: ' + JSON.stringify(cd).slice(0, 200))
await api('POST', `/contests/${contest.id}/register`, admin, { team_type: 'official' })

const waCode = `#include <cstdio>\nint main(){int x;scanf("%d",&x);printf("%d\\n",x+1);}`
const sub = await api('POST', '/submissions', admin, {
  problem_id: copyId, contest_id: contest.id, language: 'cpp', code: waCode,
})
if (!sub.id) throw new Error('submit failed: ' + JSON.stringify(sub))
let verdict = null, t0 = Date.now()
for (let i = 0; i < 120; i++) {
  await new Promise((r) => setTimeout(r, 500))
  const s = await api('GET', `/submissions/${sub.id}`, admin)
  if (!['PENDING', 'COMPILING', 'JUDGING'].includes(s.status)) {
    verdict = s; break
  }
}
if (!verdict) throw new Error('judging timeout')
let cases = verdict.cases
if (typeof cases === 'string') cases = JSON.parse(cases)
check('verdict-wa', 'WA', verdict.status)
check('first-fail-at-1', 1, cases[0].index)
check('first-fail-status', 'WA', cases[0].status)
const skipped = cases.filter((c) => c.status === 'SKIPPED').length
const real = cases.filter((c) => c.status !== 'SKIPPED').length
check('skipped-29', 29, skipped)
check('real-judged-1', 1, real)
check('elapsed-fast', true, Date.now() - t0 < 20000, )

await api('DELETE', `/problems/${prob.id}`, admin)
console.log(`\nRESULT: ${passed} passed, ${failed} failed`)
process.exit(failed ? 1 : 0)
