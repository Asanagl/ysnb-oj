// M4 E2E driver: problem package import/export round-trip + judge-source
// upload. Runs LOCALLY (Node) against the production API; credentials come
// from remote-test/.ojenv (gitignored).
//   node m4-import-export-e2e.mjs
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
  else { failed++; console.log(`FAIL ${name}: expected ${JSON.stringify(expected)} got ${JSON.stringify(actual)}`) }
}

async function api(method, apiPath, token, body) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined && !(body instanceof FormData)) headers['Content-Type'] = 'application/json'
  const r = await fetch(BASE + apiPath, {
    method, headers, body: body !== undefined ? (body instanceof FormData ? body : JSON.stringify(body)) : undefined,
  })
  const text = await r.text()
  try { return JSON.parse(text) } catch { return { raw: text.slice(0, 200) } }
}

function zipSync(entries) {
  // minimal stored (uncompressed) zip writer — CRC32 via table
  const table = new Int32Array(256).map((_, n) => {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    return c
  })
  const crc = (buf) => {
    let c = -1
    for (const b of buf) c = table[(c ^ b) & 0xff] ^ (c >>> 8)
    return (c ^ -1) >>> 0
  }
  const enc = new TextEncoder()
  const chunks = [], central = []
  let offset = 0
  for (const [name, content] of Object.entries(entries)) {
    const nameB = enc.encode(name), data = typeof content === 'string' ? enc.encode(content) : content
    const crcV = crc(data)
    const head = new Uint8Array(30 + nameB.length)
    const dv = new DataView(head.buffer)
    dv.setUint32(0, 0x04034b50, true)
    dv.setUint16(4, 20, true)
    dv.setUint16(8, 0, true) // stored
    dv.setUint32(14, crcV, true)
    dv.setUint32(18, data.length, true)
    dv.setUint32(22, data.length, true)
    dv.setUint16(26, nameB.length, true)
    head.set(nameB, 30)
    chunks.push(head, data)
    const cen = new Uint8Array(46 + nameB.length)
    const cv = new DataView(cen.buffer)
    cv.setUint32(0, 0x02014b50, true)
    cv.setUint16(4, 20, true)
    cv.setUint32(16, crcV, true)
    cv.setUint32(20, data.length, true)
    cv.setUint32(24, data.length, true)
    cv.setUint16(28, nameB.length, true)
    cv.setUint32(42, offset, true)
    cen.set(nameB, 46)
    central.push(cen)
    offset += head.length + data.length
  }
  const centralStart = offset
  let centralSize = 0
  for (const c of central) centralSize += c.length
  const end = new Uint8Array(22)
  const ev = new DataView(end.buffer)
  ev.setUint32(0, 0x06054b50, true)
  ev.setUint16(8, central.length, true)
  ev.setUint16(10, central.length, true)
  ev.setUint32(12, centralSize, true)
  ev.setUint32(16, centralStart, true)
  return new Blob([...chunks, ...central, end])
}

const admin = (await api('POST', '/auth/login', null,
  { username: env.ADMIN_USERNAME || 'admin', password: ADMIN_PW })).token
if (!admin) throw new Error('admin login failed')

// ---- 1. own-format export -> import round-trip ----
const src = await api('POST', '/problems', admin, {
  title: 'm4-roundtrip ' + Date.now().toString(36).slice(-4),
  statement_md: '<p>a+b problem</p>',
  time_limit_ms: 2000, mem_limit_mb: 512,
  judge_mode: 'spj',
  checker_source: 'INT main(){return 0;} // m4 checker marker',
  samples: [], tags: [],
})
check('create-src-problem', true, src.id !== undefined)

// upload a testdata zip so export has cases
const tdZip = zipSync({
  '1.in': '1 2\n', '1.out': '3\n',
  '2.in': '10 20\n', '2.out': '30\n',
})
{
  const form = new FormData()
  form.append('file', tdZip, 'testdata.zip')
  const r = await api('POST', `/problems/${src.id}/testdata`, admin, form)
  check('upload-testdata', 2, r.stored)
}

// checker file upload (route added in M4)
{
  const form = new FormData()
  form.append('file', new Blob(['// checker via upload\nint main(){return 0;}'], { type: 'text/plain' }), 'checker.cpp')
  const r = await api('POST', `/problems/${src.id}/checker`, admin, form)
  check('checker-upload', true, r.ok)
}
{
  const r = await api('GET', `/problems/${src.id}`, admin)
  check('checker-upload-persisted', true, (r.checker_source || '').includes('via upload'))
}

// export and inspect
{
  const r = await fetch(`${BASE}/problems/${src.id}/export`, { headers: { Authorization: `Bearer ${admin}` } })
  const buf = new Uint8Array(await r.arrayBuffer())
  check('export-content-type', 'application/zip', r.headers.get('content-type'))
  const text = new TextDecoder().decode(buf)
  check('export-has-checker', true, text.includes('checker via upload'))
  check('export-has-meta', true, text.includes('judge_mode=spj'))

  // re-import the exported zip
  const form = new FormData()
  form.append('file', new Blob([buf]), 'export.zip')
  const imp = await api('POST', '/problems/import', admin, form)
  check('import-own-format', 'own', imp.format)
  check('import-stored-2-cases', 2, imp.stored)
  check('import-judge-mode', 'spj', imp.problem?.judge_mode)
  check('import-title-kept', true, (imp.problem?.title || '').includes('m4-roundtrip'))
  check('import-hidden', 'hidden', imp.problem?.visibility)
}

// ---- 2. DOMjudge-style package ----
{
  const zip = zipSync({
    'problem.yaml': 'name: Sum Two\nlimits: {timeout: 1.5, memory: 512}\n',
    'statements/problem.nl.md': '# Som van twee\nLees twee getallen.\n',
    'data/sample/1.in': '1 2\n', 'data/sample/1.ans': '3\n',
    'data/secret/2.in': '5 6\n', 'data/secret/2.ans': '11\n',
    'data/secret/10.in': '7 8\n', 'data/secret/10.ans': '15\n',
  })
  const form = new FormData()
  form.append('file', zip, 'domjudge.zip')
  const imp = await api('POST', '/problems/import', admin, form)
  check('domjudge-title', 'Sum Two', imp.problem?.title)
  check('domjudge-time-1500', 1500, imp.problem?.time_limit_ms)
  check('domjudge-mem-512', 512, imp.problem?.mem_limit_mb)
  check('domjudge-cases-3', 3, imp.stored)
  check('domjudge-mode-default', 'default', imp.problem?.judge_mode)
}

// ---- 3. Hydro-style package (with checker → spj) ----
{
  const zip = zipSync({
    'problem.yaml': 'title: Hydro Sum\ntime: 2s\nmemory: 256m\n',
    'problem_zh.md': '# 水题\n求和。\n',
    'testdata/1.in': '1 1\n', 'testdata/1.out': '2\n',
    'testdata/2.in': '2 2\n', 'testdata/2.out': '4\n',
    'checker/checker.cpp': 'int main(){return 0;} // hydro checker',
  })
  const form = new FormData()
  form.append('file', zip, 'hydro.zip')
  const imp = await api('POST', '/problems/import', admin, form)
  check('hydro-title', 'Hydro Sum', imp.problem?.title)
  check('hydro-time-2000', 2000, imp.problem?.time_limit_ms)
  check('hydro-mode-spj', 'spj', imp.problem?.judge_mode)
  check('hydro-cases-2', 2, imp.stored)
}

// ---- 4. negative: zip without .in files ----
{
  const zip = zipSync({ 'readme.txt': 'no testdata here' })
  const form = new FormData()
  form.append('file', zip, 'empty.zip')
  const imp = await api('POST', '/problems/import', admin, form)
  check('empty-zip-rejected', true, (imp.error || '').includes('.in'))
}

console.log(`\nRESULT: ${passed} passed, ${failed} failed`)
process.exit(failed ? 1 : 0)
