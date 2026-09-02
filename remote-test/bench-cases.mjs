// Case-level parallelism benchmark: baseline (serial) vs future parallel.
// One problem with 50 cases (limit 1000ms, each case runs ~350ms), three
// solution shapes (AC / WA / TLE) in cpp and python3. Reports per-submission
// judge latency. Run: node bench-cases.mjs [--tag=baseline]
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
const TAG = process.argv.find((a) => a.startsWith('--tag='))?.slice(6) || 'run'
const CASES = 50

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

// ---------- problem: read n, print checksum of 1..n ----------
const prob = await api('POST', '/problems', admin, {
  title: `bench-cases ${TAG}`,
  statement_md: '<p>read n, print sum 1..n</p>',
  time_limit_ms: 1000, mem_limit_mb: 256,
  judge_mode: 'default', samples: [{ input: '10\n', output: '55\n', note: '' }], tags: [],
})
const pid = prob.id
if (!pid) throw new Error('problem create failed: ' + JSON.stringify(prob).slice(0, 200))

// zip with 50 cases: n=1000003 prime-ish workload, ~350ms each in C++
import { execSync } from 'child_process'
const py = path.join(here, 'tmp-bench-zip.py')
fs.writeFileSync(py, `import zipfile, sys
z = zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED)
for i in range(1, ${CASES} + 1):
    z.writestr(f'{i}.in', '1000003\\n')
    z.writestr(f'{i}.out', '500003500006\\n')
z.close()
`)
execSync(`python "${py}" "${path.join(here, 'bench-cases.zip')}"`)
{
  const form = new FormData()
  form.append('file', new Blob([fs.readFileSync(path.join(here, 'bench-cases.zip'))]), 'bench.zip')
  const up = await api('POST', `/problems/${pid}/testdata`, admin, form)
  if (up.stored !== CASES) throw new Error('testdata upload failed: ' + JSON.stringify(up))
}

// ---------- solutions ----------
const cppAC = `#include <cstdio>
volatile long long s = 0;
int main(){int n; if(scanf("%d",&n)!=1) return 0; volatile long long x=0; for(int i=1;i<=n;i++) x+=i; s=x; printf("%lld\\n", x);}
`
const cppWA = `#include <cstdio>
volatile long long s = 0;
int main(){int n; if(scanf("%d",&n)!=1) return 0; volatile long long x=0; for(int i=1;i<=n;i++) x+=i; s=x; printf("%lld\\n", x+1);}
`
const cppTLE = `#include <cstdio>
int main(){int n; scanf("%d",&n); volatile long long x=0; for(long long i=1;i<=20000000000LL;i++) x+=i; printf("%lld\\n", x);}
`
const pyAC = `n = int(input())
x = 0
for i in range(1, n + 1):
    x += i
print(x)
`

async function submitAndWait(code, lang) {
  const sub = await api('POST', '/submissions', admin, { problem_id: pid, language: lang, code })
  const sid = sub.id
  if (!sid) throw new Error('submit failed: ' + JSON.stringify(sub).slice(0, 200))
  const t0 = Date.now()
  for (let i = 0; i < 600; i++) {
    await new Promise((r) => setTimeout(r, 500))
    const s = await api('GET', `/submissions/${sid}`, admin)
    if (!['PENDING', 'COMPILING', 'JUDGING'].includes(s.status)) {
      const ms = Date.now() - t0
      return { sid, status: s.status, ms, maxTime: Math.max(...(s.cases || []).map((c) => c.time_ms || 0)) }
    }
  }
  throw new Error('timeout waiting submission ' + sid)
}

const results = []
async function bench(name, code, lang) {
  const r = await submitAndWait(code, lang)
  results.push({ name, ...r })
  console.log(`${name}: ${r.status} ${r.ms}ms (max case ${r.maxTime}ms)`)
}

// warm-up both languages (compile cache), then measured runs
console.log(`== ${TAG}: problem ${pid}, ${CASES} cases, limit 1000ms ==`)
await bench('warm-cpp', cppAC, 'cpp')
await bench('warm-py', pyAC, 'python3')

await bench('cpp-AC-1', cppAC, 'cpp')
await bench('cpp-AC-2', cppAC, 'cpp')
await bench('cpp-AC-3', cppAC, 'cpp')
await bench('cpp-WA-1', cppWA, 'cpp')
await bench('cpp-WA-2', cppWA, 'cpp')
await bench('cpp-TLE-1', cppTLE, 'cpp')
await bench('cpp-TLE-2', cppTLE, 'cpp')
await bench('py-AC-1', pyAC, 'python3')
await bench('py-AC-2', pyAC, 'python3')
await bench('py-AC-3', pyAC, 'python3')

// cleanup: delete the bench problem
await api('DELETE', `/problems/${pid}`, admin)
console.log('== bench problem cleaned ==')

// summary
const by = {}
for (const r of results.filter((r) => !r.name.startsWith('warm'))) {
  (by[r.name.replace(/-\d$/, '')] = by[r.name.replace(/-\d$/, '')] || []).push(r.ms)
}
console.log(`\n== ${TAG} summary (median of runs, ms) ==`)
for (const [k, v] of Object.entries(by)) {
  const sorted = [...v].sort((a, b) => a - b)
  const med = sorted[Math.floor(sorted.length / 2)]
  console.log(`${k}: median ${med}ms  runs [${v.join(', ')}]`)
}
fs.writeFileSync(path.join(here, `bench-${TAG}.json`), JSON.stringify(results, null, 2))
