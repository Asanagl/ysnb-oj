// E2E: teams + 组队赛 (2026-08-30).
// Flow: captain creates a team (capacity 3), pulls two members, shares a
// list, posts a notice; team board ranks; team registers into a team-mode
// contest; standings aggregate one row per team.
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

const pw = 'TeamUp#2026a'
const stamp = Date.now().toString(36).slice(-6)

const sup = (await api('POST', '/auth/login', null, { username: env.ADMIN_USERNAME || 'admin', password: env.ADMIN_PASSWORD })).data?.token
check('login-super', true, !!sup)

async function mkUser(name) {
  const inv = (await api('POST', '/admin/invite-codes', sup, { max_uses: 9 })).data.code
  await api('POST', '/auth/register', null, {
    username: name, password: pw, nickname: name,
    student_no: '2026' + Date.now().toString().slice(-6), invite_code: inv,
  })
  return (await api('POST', '/auth/login', null, { username: name, password: pw })).data.token
}

// three users
const capTok = await mkUser(`cap${stamp}`)
const m2Tok = await mkUser(`mem2${stamp}`)
const m3Tok = await mkUser(`mem3${stamp}`)
check('mk-3-users', true, !!capTok && !!m2Tok && !!m3Tok)
const capId = (await api('GET', '/auth/me', capTok)).data.id
const m2Id = (await api('GET', '/auth/me', m2Tok)).data.id
const m3Id = (await api('GET', '/auth/me', m3Tok)).data.id

// 1. create team (capacity 3, ICPC default)
const team = await api('POST', '/teams', capTok, { name: `ICPC队${stamp}`, bio: 'E2E', capacity: 3 })
check('create-team', 200, team.status)
const teamId = team.data.id

// 2. invite code join + capacity enforcement
const detail1 = (await api('GET', `/teams/${teamId}`, capTok)).data
const code = detail1.team.invite_code
const j2 = await api('POST', `/teams/join/${code}`, m2Tok)
check('join-by-code', 200, j2.status)
const j3 = await api('POST', `/teams/join/${code}`, m3Tok)
check('join-third', 200, j3.status)
const extraTok = await mkUser(`ext${stamp}`)
const j4 = await api('POST', `/teams/join/${code}`, extraTok)
check('capacity-full-rejected', 400, j4.status)

// 3. only captain manages: member tries to pull -> 403
const forbidden = await api('POST', `/teams/${teamId}/members/${m3Id}`, m2Tok)
check('member-cannot-pull', 403, forbidden.status)

// 4. notices + share list
const notice = await api('POST', `/teams/${teamId}/notices`, capTok, { content: '今晚 19:00 集训' })
check('notice-created', 200, notice.status)
// create a list by super, share it
const bank = (await api('GET', '/problems?page=1&size=20', capTok)).data.items
const p1 = bank.find((p) => p.review_status === 'approved' && p.contest_id == null)
const list = await api('POST', '/lists', sup, { title: `队单${stamp}`, description: '', problem_ids: [p1.id] })
const share = await api('POST', `/teams/${teamId}/lists`, capTok, { list_id: list.data.id })
check('share-list', 200, share.status)

// 5. team board: 3 members
const board = (await api('GET', `/teams/${teamId}/board`, capTok)).data
check('board-members', 3, board.length)

// 6. 组队赛: create a team-mode contest in the future, register by team
const now = new Date()
const start = new Date(now.getTime() + 3600_000)
const end = new Date(now.getTime() + 5 * 3600_000)
const contest = await api('POST', '/contests', sup, {
  title: `组队赛${stamp}`, description: '', visibility: 'public',
  start_time: start.toISOString(), end_time: end.toISOString(),
  team_mode: true, team_capacity: 3, require_registration: true,
})
check('create-team-contest', 200, contest.status)
const contestId = contest.data.id

// non-captain cannot register the team
const badReg = await api('POST', `/contests/${contestId}/register`, m2Tok, { team_id: teamId, team_type: 'official' })
check('member-cannot-register', 403, badReg.status)
// captain registers
const reg = await api('POST', `/contests/${contestId}/register`, capTok, { team_id: teamId, team_type: 'official' })
check('captain-registers-team', 200, reg.status)
check('registered-3-members', 3, reg.data.members)
// duplicate registration is idempotent
const reg2 = await api('POST', `/contests/${contestId}/register`, capTok, { team_id: teamId, team_type: 'official' })
check('re-register-idempotent', 200, reg2.status)
// individual registration blocked? (member already registered via team -> unique index)
const solo = await api('POST', `/contests/${contestId}/register`, m2Tok, { team_name: 'solo', team_type: 'official' })
check('member-solo-rejected', 400, solo.status)

// 7. standings aggregate: teams mode on -> team_mode flag in payload
const st = (await api('GET', `/contests/${contestId}/standings`, capTok)).data
check('standings-team-mode-flag', true, st.team_mode === true)

// 8. cleanup: delete contest & list (teams persist as demo data)
await api('DELETE', `/lists/${list.data.id}`, sup)
await api('POST', `/teams/${teamId}/leave`, m2Tok)
await api('POST', `/teams/${teamId}/leave`, m3Tok)
await api('POST', `/teams/${teamId}/leave`, capTok) // captain last => deletes team

console.log(`\n== TEAMS E2E DONE: ${pass} passed, ${fail} failed ==`)
process.exit(fail ? 1 : 0)
