// Public API E2E: anonymous read, API key lifecycle, sample protection,
// cph JSON shape, rate limiting. Targets the production /api/v1/public/*.
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
  if (opts.apiKey) headers['X-API-Key'] = opts.apiKey
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  const r = await fetch(BASE + p, { method, headers, body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined })
  let data = null
  try { data = await r.json() } catch { data = { raw: (await r.text()).slice(0, 100) } }
  return { status: r.status, data }
}

const admin = (await api('POST', '/auth/login', { body: { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD } })).data.token
if (!admin) throw new Error('admin login failed')

// 1. anonymous read: user summary by username
const me = (await api('GET', '/auth/me', { token: admin })).data
const summary = await api('GET', `/public/users/${me.username}`)
check('summary-200', 200, summary.status)
check('summary-username', me.username, summary.data.user?.username)
check('summary-activity-365', 365, summary.data.activity?.length)
check('summary-fields', true, 'recent_ac' in summary.data && 'ac_problems' in summary.data && 'by_status' in summary.data)

// 2. public problem catalog: seeded demo problem must appear
const cat = await api('GET', '/public/problems')
check('catalog-200', 200, cat.status)
check('catalog-has-demo', true, (cat.data.items || []).some((p) => p.title === 'A+B Problem'))
check('catalog-shape', true, (cat.data.items || []).every((p) => 'judge_mode' in p && 'tags' in p && 'sample_count' in p))

// 3. keyed sample + cph endpoints; anonymous must get 401
const pid = (cat.data.items || []).find((p) => p.title === 'A+B Problem')?.id
const anonSamples = await api('GET', `/public/problems/${pid}/samples`)
check('samples-anon-401', 401, anonSamples.status)
const anonCph = await api('GET', `/public/problems/${pid}/cph`)
check('cph-anon-401', 401, anonCph.status)

// 4. key lifecycle: create -> use -> revoke -> use fails
const created = await api('POST', '/admin/api-keys', { token: admin, body: { name: 'e2e-key' } })
check('key-created', true, created.data.key?.startsWith('ojk_'))
const key = created.data.key
const keyed = await api('GET', `/public/problems/${pid}/samples`, { apiKey: key })
check('samples-with-key-200', 200, keyed.status)
check('samples-shape', true, Array.isArray(keyed.data.samples) && keyed.data.samples.length >= 1)
check('samples-have-io', true, 'input' in keyed.data.samples[0] && 'output' in keyed.data.samples[0])

const cph = await api('GET', `/public/problems/${pid}/cph`, { apiKey: key })
check('cph-200', 200, cph.status)
check('cph-name', 'A+B Problem', cph.data.name)
check('cph-tests-match-samples', true, cph.data.tests?.length === keyed.data.samples.length)
check('cph-limits', true, cph.data.timeLimit === 1000 && cph.data.memoryLimit === 256)
check('cph-group', true, typeof cph.data.group === 'string')

const rev = await api('POST', `/admin/api-keys/${created.data.id}/revoke`, { token: admin })
check('revoke-ok', true, rev.data.ok)
const revokedUse = await api('GET', `/public/problems/${pid}/samples`, { apiKey: key })
check('revoked-401', 401, revokedUse.status)

// 5. keyed list shows audit fields
const list = await api('GET', '/admin/api-keys', { token: admin })
check('key-listed', true, (list.data || []).some((k) => k.id === created.data.id && k.prefix?.startsWith('ojk_')))
check('key-not-echoed', true, (list.data || []).every((k) => !('key_hash' in k) && !('key' in k)))

console.log(`\nRESULT: ${passed} passed, ${failed} failed`)
process.exit(failed ? 1 : 0)