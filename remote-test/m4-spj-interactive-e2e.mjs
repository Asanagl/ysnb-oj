// M4 E2E driver: SPJ + interactive end-to-end on production. Creates one
// SPJ problem (checker accepts any output equal to input reversed) and one
// interactive problem (interactor plays guess-the-number), submits correct
// and wrong solutions, and asserts verdicts. Uses .ojenv credentials.
//   node m4-spj-interactive-e2e.mjs
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

async function login(u, p) {
  return (await api('POST', '/auth/login', null, { username: u, password: p })).token
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const admin = await login(env.ADMIN_USERNAME || 'admin', env.ADMIN_PASSWORD)
if (!admin) throw new Error('admin login failed')

// ---- 1. SPJ problem: checker tests ./checker in ans out; AC iff output
// equals the input echoed back (permissive check that only a custom checker
// can do — standard diff against .out would judge the same output WA since
// answer files are deliberately different from what solutions produce). ----
const spjChecker = `#include <cstdio>
#include <cstring>
int main(int argc, char** argv) {
    if (argc < 4) return 1;
    FILE* fin = fopen(argv[1], "r");
    FILE* fout = fopen(argv[3], "r");
    if (!fin || !fout) return 1;
    long long a = -1, b = -1;
    fscanf(fin, "%lld", &a);
    int n = fscanf(fout, "%lld", &b);
    if (n != 1 || a != b) { fprintf(stderr, "expected %lld got %lld\\n", a, b); return 1; }
    return 0;
}
`

// simpler: upload cases via single-case endpoint
const spj = await api('POST', '/problems', admin, {
  title: 'm4-spj-echo ' + Date.now().toString(36).slice(-4),
  statement_md: '<p>output the number you read</p>',
  time_limit_ms: 1000, mem_limit_mb: 256,
  judge_mode: 'spj', checker_source: spjChecker, samples: [], tags: [],
})
check('create-spj-problem', true, spj.id !== undefined)

async function uploadCase(problemId, idx, inData, outData) {
  // local python builds the <n>.in/<n>.out zip; windows "python" resolves
  // via PATH, unix falls back below if needed.
  const entries = [['1.in', inData]]
  if (outData !== null) entries.push(['1.out', outData])
  const py = 'import zipfile,sys\nz=zipfile.ZipFile(sys.argv[1],"w")\n' +
    entries.map(([n, c]) => `z.writestr(${JSON.stringify(n)}, ${JSON.stringify(c)})`).join('\n') +
    '\nz.close()'
  const pyFile = path.join(here, 'tmp-mkzip.py')
  fs.writeFileSync(pyFile, py)
  execSync(`python "${pyFile}" "${path.join(here, 'out.zip')}"`)
  fs.unlinkSync(pyFile)
  const form = new FormData()
  form.append('file', new Blob([fs.readFileSync(path.join(here, 'out.zip'))]), 'out.zip')
  return api('POST', `/problems/${problemId}/testdata`, admin, form)
}

const r1 = await uploadCase(spj.id, 1, '42\n', '999\n') // answer deliberately wrong; checker decides
check('upload-spj-case', 1, r1.stored)

// ---- 2. interactive problem: interactor prints a number, user echoes it ----
const interactor = `#include <cstdio>
int main(int argc, char** argv) {
    FILE* fin = fopen(argv[1], "r");
    if (!fin) return 2;
    long long secret = 0;
    if (fscanf(fin, "%lld", &secret) != 1) return 2;
    printf("%lld\\n", secret);
    fflush(stdout);
    long long reply = -1;
    if (scanf("%lld", &reply) != 1) { fprintf(stderr, "no reply\\n"); return 1; }
    if (reply != secret) { fprintf(stderr, "wrong echo\\n"); return 1; }
    return 0;
}
`
const ita = await api('POST', '/problems', admin, {
  title: 'm4-interactive-echo ' + Date.now().toString(36).slice(-4),
  statement_md: '<p>the judge sends a number; echo it back</p>',
  time_limit_ms: 1000, mem_limit_mb: 256,
  judge_mode: 'interactive', interactor_source: interactor, samples: [], tags: [],
})
check('create-interactive-problem', true, ita.id !== undefined)
const r2 = await uploadCase(ita.id, 1, '7\n', null) // interactive: no .out
check('upload-interactive-case', 1, r2.stored)

// ---- 3. submissions ----
const submit = async (pid, code) => {
  const s = await api('POST', '/submissions', admin, { problem_id: pid, language: 'cpp', code })
  return s.id
}
const waitVerdict = async (sid) => {
  for (let i = 0; i < 40; i++) {
    await sleep(1500)
    const s = await api('GET', `/submissions/${sid}`, admin)
    if (!['PENDING', 'COMPILING', 'JUDGING'].includes(s.status)) return s
  }
  return { status: 'TIMEOUT-WAITING' }
}

const acAC = await submit(spj.id, '#include <cstdio>\nint main(){long long x;scanf("%lld",&x);printf("%lld\\n",x);}\n')
const spjAC = await waitVerdict(acAC)
check('spj-ac', 'AC', spjAC.status)

const spjWrong = await submit(spj.id, '#include <cstdio>\nint main(){long long x;scanf("%lld",&x);printf("%lld\\n",x+1);}\n')
const spjWA = await waitVerdict(spjWrong)
check('spj-wa', 'WA', spjWA.status)

const itaAC = await submit(ita.id, '#include <cstdio>\nint main(){long long x;scanf("%lld",&x);printf("%lld\\n",x);}\n')
const itaACV = await waitVerdict(itaAC)
check('interactive-ac', 'AC', itaACV.status)

const itaWrong = await submit(ita.id, '#include <cstdio>\nint main(){printf("0\\n");return 0;}\n')
const itaWA = await waitVerdict(itaWrong)
check('interactive-wa', 'WA', itaWA.status)

console.log(`\nRESULT: ${passed} passed, ${failed} failed`)
process.exit(failed ? 1 : 0)
