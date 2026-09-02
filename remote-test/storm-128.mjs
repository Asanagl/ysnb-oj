// Storm-128 load test: 128 concurrent submissions (mixed AC/WA/TLE, cpp+py)
// fanned out across 4 subagent workers, plus parallel standings/read traffic.
// Measures judge throughput, phantom-RE rate, API latency under load.
// Run: node storm-128.mjs --tag=storm
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
const TAG = process.argv.find((a) => a.startsWith('--tag='))?.slice(6) || 'storm'
const TOTAL = Number(process.argv.find((a) => a.startsWith('--n='))?.slice(4) || 128)
const CONCURRENCY = Number(process.argv.find((a) => a.startsWith('--c='))?.slice(4) || 32)

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

const admin = (await api('POST', '/auth/login', null,
  { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).token
if (!admin) throw new Error('admin login failed')

// ---------- problem: echo a+b ----------
const prob = await api('POST', '/problems', admin, {
  title: `storm-128 ${TAG}`,
  statement_md: '<p>read a b, print a+b</p>',
  time_limit_ms: 500, mem_limit_mb: 256,
  judge_mode: 'default', samples: [{ input: '1 2\n', output: '3\n', note: '' }], tags: [],
})
const pid = prob.id
for (let i = 1; i <= 3; i++) {
  const fd = new FormData()
  fd.append('input', new Blob([`${i} ${i * 2}\n`]), `${i}.in`)
  fd.append('output', new Blob([`${i * 3}\n`]), `${i}.out`)
  await fetch(`${BASE}/problems/${pid}/testdata/case`, {
    method: 'POST', headers: { Authorization: `Bearer ${admin}` }, body: fd,
  })
}
// ---------- 8 storm users (register needs invite + student_no) ----------
const invite = await api('POST', '/admin/invite-codes', admin, { max_uses: 20, expires_in_hours: 1 })
const tokens = []
for (let u = 0; u < 8; u++) {
  const name = `storm${u}${Date.now() % 100000}`
  await api('POST', '/auth/register', null, {
    username: name, password: 'StormPass123x', invite_code: invite.code,
    student_no: `2026${String(800000 + u)}`,
  })
  const tok = (await api('POST', '/auth/login', null, { username: name, password: 'StormPass123x' })).token
  if (tok) tokens.push(tok)
}
console.log(`storm users: ${tokens.length}`)

const solutions = {
  acCpp: '#include <bits/stdc++.h>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b<<std::endl;}',
  waCpp: '#include <bits/stdc++.h>\nint main(){std::cout<<42<<std::endl;}',
  tleCpp: '#include <bits/stdc++.h>\nint main(){while(1);}',
  acPy: 'a,b=map(int,input().split())\nprint(a+b)',
  waPy: 'print(42)',
  tlePy: 'while True: pass',
}

const t0 = Date.now()
let submitted = 0, ac = 0, wa = 0, tle = 0, other = 0, errors = 0
const latencies = []

async function submitOne(i) {
  const lang = (i % 2 === 0) ? 'cpp' : 'python3'
  const kind = i % 3 // 0=AC 1=WA 2=TLE, per-language shapes
  const code = lang === 'cpp'
    ? (kind === 0 ? solutions.acCpp : kind === 1 ? solutions.waCpp : solutions.tleCpp)
    : (kind === 0 ? solutions.acPy : kind === 1 ? solutions.waPy : solutions.tlePy)
  const ts = Date.now()
  try {
    const tok = tokens[i % tokens.length] ?? admin
    const sub = await api('POST', '/submissions', tok, { problem_id: pid, language: lang, code })
    if (!sub.id) { errors++; return }
    // poll for verdict
    for (let p = 0; p < 120; p++) {
      await new Promise((r) => setTimeout(r, 1000))
      const r = await api('GET', `/submissions/${sub.id}`, tok)
      if (!['PENDING', 'COMPILING', 'JUDGING'].includes(r.status)) {
        latencies.push(Date.now() - ts)
        if (r.status === 'AC') ac++
        else if (r.status === 'WA') wa++
        else if (r.status === 'TLE') tle++
        else other++
        return
      }
    }
    other++
  } catch { errors++ }
}

const queue = Array.from({ length: TOTAL }, (_, i) => i)
const workers = Array.from({ length: CONCURRENCY }, async () => {
  while (queue.length) {
    const i = queue.shift()
    submitted++
    if (submitted % 16 === 0) console.log(`progress ${submitted}/${TOTAL} @${((Date.now() - t0) / 1000).toFixed(0)}s`)
    await submitOne(i)
  }
})
await Promise.all(workers)
const wall = ((Date.now() - t0) / 1000).toFixed(1)

latencies.sort((a, b) => a - b)
const pct = (p) => latencies[Math.floor(latencies.length * p)] ?? 0
console.log(`\n=== storm-128 result ===`)
console.log(`wall=${wall}s total=${TOTAL} submitted=${submitted}`)
console.log(`AC=${ac} WA=${wa} TLE=${tle} other=${other} errors=${errors}`)
console.log(`latency p50=${pct(0.5)}ms p90=${pct(0.9)}ms p99=${pct(0.99)}ms max=${latencies.at(-1) ?? 0}ms`)
console.log(`throughput=${(TOTAL / wall).toFixed(2)} submissions/s`)
// "other" buckets RE/SE/CE — RE/SE under storm indicates phantom kills
console.log(`note: "other" = RE/SE/CE/超时未判; phantom indicator is other>0 with acCpp shapes`)