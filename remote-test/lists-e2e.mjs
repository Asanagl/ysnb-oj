// E2E: training problem lists (题单) (2026-08-30).
// Flow: setter creates a list with 2 problems -> all logged-in users see it
// (site-wide public); a normal user who submitted AC on one problem sees
// progress ac/todo; update reorders; delete cleans up.
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

const pw = 'Lists#2026a'

const sup = (await api('POST', '/auth/login', null, { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).data?.token
check('login-super', true, !!sup)

// fresh ordinary user
const uname = `lstuser${Date.now().toString(36).slice(-5)}`
const inv = (await api('POST', '/admin/invite-codes', sup, { max_uses: 5 })).data.code
await api('POST', '/auth/register', null, {
  username: uname, password: pw, nickname: 'lst', student_no: '2026' + Date.now().toString().slice(-6), invite_code: inv,
})
const user = (await api('POST', '/auth/login', null, { username: uname, password: pw })).data.token
check('login-user', true, !!user)

// pick two approved bank problems
const bank = (await api('GET', '/problems?page=1&size=50', user)).data.items
const ps = bank.filter((p) => p.review_status === 'approved' && p.contest_id == null).slice(0, 2)
check('two-problems', true, ps.length === 2)

// create list (super acts as setter)
const created = await api('POST', '/lists', sup, {
  title: `E2E题单 ${Date.now().toString(36)}`,
  description: 'E2E 自动创建',
  problem_ids: ps.map((p) => p.id),
  notes: { [ps[0].id]: '必做' },
})
check('create-list', 200, created.status)
const listId = created.data.id

// appears in the all-lists view for the ordinary user
const all = (await api('GET', '/lists', user)).data
check('listed-public', true, all.some((l) => l.id === listId))

// detail: order preserved, editable=false for ordinary user
const detail = (await api('GET', `/lists/${listId}`, user)).data
check('detail-count', 2, detail.items.length)
check('detail-order', ps[0].id, detail.items[0].item.problem_id)
check('detail-note', '必做', detail.items[0].item.note)
check('detail-not-editable', false, detail.editable)

// progress: todo for both initially
check('progress-todo', 'todo', detail.items[0].progress)

// user submits on problem 1 → progress becomes tried (likely CE/WA, not AC)
const langs = (await api('GET', '/languages', user)).data
const langId = langs[0]?.id ?? langs.languages?.[0]?.id
const sub = await api('POST', '/submissions', user, { problem_id: ps[0].id, language: langId, code: 'int main(){return 0;}' })
check('submit-ok', true, !!sub.data?.id)

// wait briefly for judging, then read progress again (accept tried or ac)
let prog = 'todo'
for (let i = 0; i < 20; i++) {
  await new Promise((r) => setTimeout(r, 1500))
  const d = (await api('GET', `/lists/${listId}`, user)).data
  prog = d.items[0].progress
  if (prog !== 'todo') break
}
check('progress-after-submit', true, prog === 'tried' || prog === 'ac')

// update: swap order
const upd = await api('PUT', `/lists/${listId}`, sup, {
  title: created.data.title, description: 'updated',
  problem_ids: [ps[1].id, ps[0].id],
})
check('update-list', 200, upd.status)
const detail2 = (await api('GET', `/lists/${listId}`, user)).data
check('updated-order', ps[1].id, detail2.items[0].item.problem_id)
check('updated-desc', 'updated', detail2.list.description)

// anonymous can't list (login-gated community)
const anon = await api('GET', '/lists', null)
check('anon-401', 401, anon.status)

// ordinary user cannot create
const forbidden = await api('POST', '/lists', user, { title: 'x', description: '', problem_ids: [] })
check('user-create-403', 403, forbidden.status)

// cleanup
const del = await api('DELETE', `/lists/${listId}`, sup)
check('delete-list', 200, del.status)
const gone = (await api('GET', `/lists/${listId}`, sup))
check('deleted-404', 404, gone.status)

console.log(`\n== LISTS E2E DONE: ${pass} passed, ${fail} failed ==`)
process.exit(fail ? 1 : 0)
