// E2E driver for user-problem + review + solutions. Runs LOCALLY (Node) and
// targets the production API over HTTP; credentials come from remote-test
// .sshenv-style env file oj.env.local (gitignored), never embedded here.
//   node user-problem-e2e.mjs
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
const ADMIN_PW = env.ADMIN_PASSWORD
if (!ADMIN_PW) throw new Error('ADMIN_PASSWORD missing from .ojenv')

let passed = 0, failed = 0
function check(name, expected, actual) {
  if (expected === actual) { passed++; console.log(`PASS ${name}: ${actual}`) }
  else { failed++; console.log(`FAIL ${name}: expected ${expected} got ${actual}`) }
}

async function api(method, path, token, body) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const r = await fetch(BASE + path, {
    method, headers, body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await r.text()
  try { return JSON.parse(text) } catch { return { raw: text.slice(0, 200) } }
}

const admin = (await api('POST', '/auth/login', null,
  { username: env.ADMIN_USERNAME || 'admin', password: ADMIN_PW })).token
if (!admin) throw new Error('admin login failed')

// 1. author registers with a runtime-generated password
const authorPw = crypto.randomUUID().replaceAll('-', '').slice(0, 12) + 'A1'
const uname = 'author' + Date.now().toString(36).slice(-5)
const code = (await api('POST', '/admin/invite-codes', admin, { max_uses: 5 })).code
check('register-author', true,
  (await api('POST', '/auth/register', null, {
    username: uname, password: authorPw, nickname: 'chuti',
    student_no: '2026' + Date.now().toString().slice(-6), invite_code: code,
  })).ok)
const author = (await api('POST', '/auth/login', null,
  { username: uname, password: authorPw })).token

// 2. author creates a problem (hidden + pending)
const created = await api('POST', '/my/problems', author, {
  title: 'user-problem-e2e ' + uname, statement_md: '<p>a+b</p>',
  input_desc: 'two ints', output_desc: 'sum',
  time_limit_ms: 1000, mem_limit_mb: 256,
  judge_mode: 'default', tags: [], samples: [],
})
const pid = created.id
check('user-problem-created', true, pid !== undefined)
check('user-problem-hidden', 'hidden', created.visibility)
check('user-problem-pending', 'pending', created.review_status)

// 3. not in public bank
const bank = await api('GET', '/problems?size=100', author)
check('not-in-public-bank', false, bank.items.some((p) => p.id === pid))

// 4. hidden problem not submittable (404 before the fix; now the author CAN
// see their own pending problem, so the guard moved into createSubmission —
// expect the review-flow rejection message instead)
check('hidden-not-submittable', '题目尚未通过审核，暂不能提交',
  (await api('POST', '/submissions', author,
    { problem_id: pid, language: 'cpp', code: 'int main(){}' })).error)

// 5. admin approves
const pending = await api('GET', '/admin/problems/pending', admin)
check('in-pending-queue', true, pending.some((p) => p.id === pid))
await api('POST', `/admin/problems/${pid}/review`, admin, { action: 'approve' })
const approved = await api('GET', `/problems/${pid}`, author)
check('approved-visible', pid, approved.problem.id)
check('approved-status', 'approved', approved.problem.review_status)
check('approved-members-visibility', 'members', approved.problem.visibility)

// 6. solutions flow
const sol = await api('POST', `/problems/${pid}/solutions`, author,
  { title: 'official', body_md: '<p>print a+b, O(1)</p>' })
const sid = sol.id
check('solution-created', true, sid !== undefined)
await api('PUT', `/problems/${pid}/solutions/${sid}`, author,
  { title: 'official', body_md: '<p>print a+b</p>', is_official: true })
let sols = (await api('GET', `/problems/${pid}/solutions`, author)).solutions
check('official-pinned', true, sols.find((s) => s.id === sid).is_official)
check('reply-created', true,
  (await api('POST', `/problems/${pid}/solutions`, admin,
    { title: '', body_md: '<p>good!</p>', parent_id: sid })).id !== undefined)
sols = (await api('GET', `/problems/${pid}/solutions`, author)).solutions
check('reply-listed', 2, sols.length)
check('delete-solution', true,
  (await api('DELETE', `/problems/${pid}/solutions/${(await api('POST', `/problems/${pid}/solutions`, admin, { title: '', body_md: '<p>temp</p>' })).id}`, admin)).ok)

console.log(`\n== E2E DONE: ${passed} passed, ${failed} failed ==`)
process.exit(failed > 0 ? 1 : 0)
