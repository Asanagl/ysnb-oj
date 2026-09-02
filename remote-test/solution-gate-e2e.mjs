// E2E: solution-area unlock gate (2026-08-30).
// Rules under test (user-confirmed):
//   1. fresh user, no submission  -> GET/POST solutions 403 (locked)
//   2. after ANY submission       -> unlocked (200)
//   3. admin exempt               -> always 200
// Uses an approved public problem. The submission may judge (daemon live);
// we only assert HTTP status, not verdicts.
import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const fs = require('fs')
const env = Object.fromEntries(
  fs.readFileSync(new URL('./.ojenv', import.meta.url), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)
const BASE = (env.BASE || 'http://localhost:8080').replace(/\/api\/v1\/?$/, '')

let pass = 0, fail = 0
function check(name, want, got) {
  const ok = String(got) === String(want)
  console.log(`${ok ? 'PASS' : 'FAIL'} ${name}: ${ok ? '' : `expected ${want} `}got ${JSON.stringify(got)}`)
  ok ? pass++ : fail++
}

async function api(method, path, token, body) {
  const r = await fetch(BASE + '/api/v1' + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  let data = null
  try { data = await r.json() } catch { /* empty */ }
  return { status: r.status, data }
}

const uname = `solgate${Date.now() % 100000}`
const aTok = (await api('POST', '/auth/login', null, { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).data?.token
check('login-admin', true, !!aTok)
const inv = await api('POST', '/admin/invite-codes', aTok, { max_uses: 5 })
const invite = inv.data?.code
check('make-invite', true, !!invite)
const reg = await api('POST', '/auth/register', null, {
  username: uname, password: 'SolGate#2026a', nickname: 'solgate',
  student_no: '2026' + Date.now().toString().slice(-6), invite_code: invite,
})
check('register-user', reg.data?.ok ?? false, true)
const uTokFinal = (await api('POST', '/auth/login', null, { username: uname, password: 'SolGate#2026a' })).data?.token
check('login-user', true, !!uTokFinal)

// pick an approved public problem (first in the bank)
const bank = await api('GET', '/problems?page=1&size=50', uTokFinal)
const prob = bank.data?.items?.find((p) => p.review_status === 'approved' && p.contest_id == null)
check('pick-public-problem', true, !!prob?.id)

// 1. locked before any submission
const pre1 = await api('GET', `/problems/${prob.id}/solutions`, uTokFinal)
check('locked-get-403', 403, pre1.status)
const pre2 = await api('POST', `/problems/${prob.id}/solutions`, uTokFinal, { body_md: 'x' })
check('locked-post-403', 403, pre2.status)
check('locked-message', '题解区未解锁：请先提交本题（比赛结束后自动开放）', pre2.data?.error)

// 2. admin exempt (sees the same area unlocked)
const adm = await api('GET', `/problems/${prob.id}/solutions`, aTok)
check('admin-exempt-200', 200, adm.status)

// 3. after a submission, unlocked — pick a language the judge supports
const langs = await api('GET', '/languages', uTokFinal)
const langId = langs.data?.[0]?.id ?? langs.data?.languages?.[0]?.id
const sub = await api('POST', '/submissions', uTokFinal, { problem_id: prob.id, language: langId, code: 'int main(){return 0;}' })
check('submission-created', true, !!sub.data?.id)

const post = await api('GET', `/problems/${prob.id}/solutions`, uTokFinal)
check('unlocked-get-200', 200, post.status)
const post2 = await api('POST', `/problems/${prob.id}/solutions`, uTokFinal, { body_md: '解锁后发帖 E2E' })
check('unlocked-post-200', 200, post2.status)
if (post2.data?.id) await api('DELETE', `/problems/${prob.id}/solutions/${post2.data.id}`, uTokFinal)

console.log(`\n== SOLUTION GATE E2E DONE: ${pass} passed, ${fail} failed ==`)
process.exit(fail ? 1 : 0)
