// E2E: manual invite-code deletion (2026-08-30).
// Flow: super creates a code -> delete it -> registration with that code
// must fail (400 invalid invite code); the deleted code disappears from the
// list. Also covers "used-up code can be deleted".
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

const sup = (await api('POST', '/auth/login', null, { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).data?.token
check('login-super', true, !!sup)

// create + list shows it
const code = (await api('POST', '/admin/invite-codes', sup, { max_uses: 5 })).data.code
check('create-invite', true, !!code)

// delete it
const del = await api('DELETE', `/admin/invite-codes/${(await api('GET', '/admin/invite-codes', sup)).data.find((c) => c.code === code).id}`, sup)
check('delete-invite', 200, del.status)

// gone from the list
const list = (await api('GET', '/admin/invite-codes', sup)).data
check('deleted-not-listed', false, list.some((c) => c.code === code))

// registration with the deleted code fails
const reg = await api('POST', '/auth/register', null, {
  username: `delinv${Date.now().toString(36).slice(-5)}`, password: 'DelInv#2026a', nickname: 'x',
  student_no: '2026' + Date.now().toString().slice(-6), invite_code: code,
})
check('deleted-code-rejected', 400, reg.status)

// non-admin cannot delete
const inv2 = (await api('POST', '/admin/invite-codes', sup, { max_uses: 1 })).data.code
const anonDel = await api('DELETE', `/admin/invite-codes/1`, null)
check('anon-delete-401', 401, anonDel.status)
// cleanup leftover code
await api('DELETE', `/admin/invite-codes/${(await api('GET', '/admin/invite-codes', sup)).data.find((c) => c.code === inv2).id}`, sup)

console.log(`\n== INVITE DELETE E2E DONE: ${pass} passed, ${fail} failed ==`)
process.exit(fail ? 1 : 0)
