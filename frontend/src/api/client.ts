import axios from 'axios'

// why a global timeout: without it a stalled request leaves pages spinning
// forever; 20s covers slow judge-status polling with margin to spare.
export const api = axios.create({ baseURL: '/api/v1', timeout: 20000 })

api.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('oj_token')
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

api.interceptors.response.use(
  (resp) => resp,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('oj_token')
      localStorage.removeItem('oj_user')
    }
    return Promise.reject(err)
  },
)

export function errMsg(e: unknown): string {
  const anyErr = e as { response?: { data?: { error?: string } } }
  return anyErr.response?.data?.error ?? String(e)
}

// ---- typed API surface ----
export interface User {
  id: number
  username: string
  nickname: string
  role: string
  student_no?: string
}

export interface Problem {
  id: number
  title: string
  statement_md: string
  input_desc: string
  output_desc: string
  hint: string
  source: string
  tags: string
  samples: string
  time_limit_ms: number
  mem_limit_mb: number
  visibility: string
  judge_mode: string
  created_by: number
  contest_id?: number | null
  review_status?: string
}

export interface ProblemListSummary {
  id: number
  title: string
  description: string
  created_by: number
  created_at: string
  problem_count: number
}

export interface ProblemListItemView {
  item: { id: number; list_id: number; problem_id: number; order_index: number; note: string }
  id: number
  title: string
  time_limit_ms: number
  mem_limit_mb: number
  visibility: string
  progress: 'todo' | 'tried' | 'ac'
}

export interface Sample {
  input: string
  output: string
  note?: string
}

export interface CaseResult {
  index: number
  status: string
  time_ms: number
  mem_kb: number
  message?: string
}

export interface Submission {
  id: number
  user_id: number
  problem_id: number
  contest_id: number | null
  language: string
  code_size: number
  status: string
  time_ms: number
  memory_kb: number
  compile_message?: string
  cases: CaseResult[]
  created_at: string
  judged_at: string | null
  code?: string
  cancelled?: boolean
  is_practice?: boolean
}

export interface Contest {
  id: number
  title: string
  description: string
  mode: string
  visibility: string
  start_time: string
  end_time: string
  freeze_time: string | null
  manual_frozen?: boolean
  manual_frozen_at?: string | null
  freeze_enabled?: boolean
  no_manual_freeze?: boolean
  require_registration?: boolean | null
  reveal_count?: number
  created_by: number
}

export interface ContestRegistration {
  id: number
  contest_id: number
  user_id: number
  username?: string
  team_name: string
  team_type: string
  created_at: string
}

export interface ContestNotice {
  id: number
  content: string
  created_at: string
}

export interface ContestUserFlag {
  id: number
  contest_id: number
  user_id: number
  username?: string
  starred: boolean
  cheated: boolean
  updated_at?: string
}

export interface UserBrief {
  id: number
  username: string
  nickname: string
}

export interface ContestProblemItem {
  label: string
  id: number
  title: string
  time_limit_ms: number
  mem_limit_mb: number
}

export interface StandingRow {
  user_id: number
  username: string
  solved: number
  penalty_ms: number
  rank: number
  starred: boolean
  cheated: boolean
  // IOI contests: solved = 题目全对数, penalty_ms carries the TOTAL SCORE
  // (backend reuses the slot; the scoreboard renders it as 总分 in IOI mode),
  // and each cell's solved_ms holds that problem's best partial score.
  cells: Record<string, { label: string; attempts: number; solved_ms: number; pending: number; solved: boolean }>
}

export interface UserProfile {
  user: { id: number; username: string; nickname: string; role: string; created_at: string }
  by_status: Record<string, number>
  ac_problems: number
  tried_problems: number
  activity: { date: string; submissions: number; ac: number }[]
  trend: { date: string; submissions: number; ac: number }[]
  tags: { tag: string; solved: number; attempted: number }[]
  // per-platform daily buckets from the external practice sync; the profile
  // heatmap merges 本站 + any selected subset of these platforms.
  platforms?: {
    date: string
    submissions: number
    ac: number
    by_platform: Record<string, { submissions: number; ac: number }>
  }[]
}

export const Users = {
  async profile(id: number | string) {
    const r = await api.get(`/users/${id}/profile`)
    return r.data as UserProfile
  },
}

export interface ProblemSolution {
  id: number
  problem_id: number
  user_id: number
  username: string
  parent_id: number | null
  title: string
  body_md: string
  is_official: boolean
  created_at: string
}

export const Solutions = {
  async list(problemId: number | string) {
    const r = await api.get(`/problems/${problemId}/solutions`)
    return (r.data as { solutions: ProblemSolution[] }).solutions
  },
  async create(problemId: number | string, body: { title: string; body_md: string; parent_id?: number }) {
    const r = await api.post(`/problems/${problemId}/solutions`, body)
    return r.data
  },
  async update(problemId: number | string, sid: number, body: { title: string; body_md: string; is_official?: boolean }) {
    const r = await api.put(`/problems/${problemId}/solutions/${sid}`, body)
    return r.data
  },
  async remove(problemId: number | string, sid: number) {
    const r = await api.delete(`/problems/${problemId}/solutions/${sid}`)
    return r.data
  },
}

export const MyProblems = {
  async list() {
    const r = await api.get('/my/problems')
    return r.data as Problem[]
  },
  async create(payload: Record<string, unknown>) {
    const r = await api.post('/my/problems', payload)
    return r.data as Problem
  },
  async update(id: number, payload: Record<string, unknown>) {
    const r = await api.put(`/my/problems/${id}`, payload)
    return r.data as Problem
  },
  async pending() {
    const r = await api.get('/admin/problems/pending')
    return r.data as Problem[]
  },
  async review(id: number, action: 'approve' | 'reject') {
    const r = await api.post(`/admin/problems/${id}/review`, { action })
    return r.data as Problem
  },
}

export const Auth = {
  async login(username: string, password: string) {
    const r = await api.post('/auth/login', { username, password })
    return r.data as { token: string; user: User }
  },
  async register(payload: { username: string; password: string; nickname: string; student_no: string; invite_code: string }) {
    const r = await api.post('/auth/register', payload)
    return r.data
  },
  async me() {
    const r = await api.get('/auth/me')
    return r.data as User
  },
}

export const Lists = {
  async all() {
    const r = await api.get('/lists')
    return r.data as ProblemListSummary[]
  },
  async get(id: number | string) {
    const r = await api.get(`/lists/${id}`)
    return r.data as { list: ProblemListSummary; items: ProblemListItemView[]; editable: boolean }
  },
  async create(payload: { title: string; description: string; problem_ids: number[]; notes?: Record<number, string> }) {
    const r = await api.post('/lists', payload)
    return r.data as ProblemListSummary
  },
  async update(id: number, payload: { title: string; description: string; problem_ids: number[]; notes?: Record<number, string> }) {
    const r = await api.put(`/lists/${id}`, payload)
    return r.data as ProblemListSummary
  },
  async remove(id: number) {
    const r = await api.delete(`/lists/${id}`)
    return r.data
  },
}

export interface TeamSummary {
  id: number
  name: string
  bio: string
  capacity: number
  captain_id: number
  captain: string
  invite_code: string
  member_count: number
  role: 'captain' | 'member' | 'none'
  created_at: string
}

export interface TeamMemberView {
  user_id: number
  username: string
  nickname: string
  is_captain: boolean
}

export interface TeamDetail {
  team: TeamSummary & { members: TeamMemberView[] }
  notices: { id: number; content: string; created_at: string }[]
  lists: { id: number; title: string; description: string; items: number }[]
}

export interface TeamBoardRow {
  user_id: number
  username: string
  nickname: string
  ac: number
  tried: number
}

export const Teams = {
  async all() {
    const r = await api.get('/teams')
    return r.data as TeamSummary[]
  },
  async create(payload: { name: string; bio: string; capacity: number }) {
    const r = await api.post('/teams', payload)
    return r.data as TeamSummary
  },
  async get(id: number | string) {
    const r = await api.get(`/teams/${id}`)
    return r.data as TeamDetail
  },
  async join(code: string) {
    const r = await api.post(`/teams/join/${encodeURIComponent(code)}`)
    return r.data as { ok: boolean; team_id: number }
  },
  async leave(id: number) {
    const r = await api.post(`/teams/${id}/leave`)
    return r.data
  },
  async pull(id: number, uid: number) {
    const r = await api.post(`/teams/${id}/members/${uid}`)
    return r.data
  },
  async kick(id: number, uid: number) {
    const r = await api.delete(`/teams/${id}/members/${uid}`)
    return r.data
  },
  async transfer(id: number, uid: number) {
    const r = await api.post(`/teams/${id}/captain/${uid}`)
    return r.data
  },
  async regenerateInvite(id: number) {
    const r = await api.post(`/teams/${id}/invite`)
    return r.data
  },
  async createNotice(id: number, content: string) {
    const r = await api.post(`/teams/${id}/notices`, { content })
    return r.data
  },
  async deleteNotice(id: number, nid: number) {
    const r = await api.delete(`/teams/${id}/notices/${nid}`)
    return r.data
  },
  async board(id: number | string) {
    const r = await api.get(`/teams/${id}/board`)
    return r.data as TeamBoardRow[]
  },
  async shareList(id: number, listId: number) {
    const r = await api.post(`/teams/${id}/lists`, { list_id: listId })
    return r.data
  },
  async unshareList(id: number, listId: number) {
    const r = await api.delete(`/teams/${id}/lists/${listId}`)
    return r.data
  },
}

export const Problems = {
  async list(params: Record<string, unknown>) {
    const r = await api.get('/problems', { params })
    return r.data as { total: number; items: Problem[] }
  },
  async get(id: number | string) {
    const r = await api.get(`/problems/${id}`)
    return r.data as { problem: Problem; samples: Sample[]; case_count: number; can_manage: boolean; checker_source?: string; interactor_source?: string }
  },
  async create(payload: Record<string, unknown>) {
    const r = await api.post('/problems', payload)
    return r.data as Problem
  },
  async update(id: number, payload: Record<string, unknown>) {
    const r = await api.put(`/problems/${id}`, payload)
    return r.data
  },
  async remove(id: number) {
    const r = await api.delete(`/problems/${id}`)
    return r.data
  },
  async uploadTestdata(id: number, file: File) {
    const form = new FormData()
    form.append('file', file)
    const r = await api.post(`/problems/${id}/testdata`, form)
    return r.data as { stored: number }
  },
  async testdata(id: number) {
    const r = await api.get(`/problems/${id}/testdata`)
    return r.data as { id: number; index: number; input_size: number; answer_size: number }[]
  },
  async copy(id: number) {
    const r = await api.post(`/problems/${id}/copy`)
    return r.data as Problem
  },
  async importPackage(file: File) {
    const form = new FormData()
    form.append('file', file)
    const r = await api.post('/problems/import', form)
    return r.data as { problem: Problem; stored: number; format: string; notes: string[] }
  },
  async uploadJudgeSource(id: number, kind: 'checker' | 'interactor', file: File) {
    const form = new FormData()
    form.append('file', file)
    const r = await api.post(`/problems/${id}/${kind}`, form)
    return r.data as { ok: boolean; filename: string; bytes: number }
  },
  async uploadCase(id: number, inputFile: File, outputFile?: File) {
    const form = new FormData()
    form.append('input', inputFile)
    if (outputFile) form.append('output', outputFile)
    const r = await api.post(`/problems/${id}/testdata/case`, form)
    return r.data
  },
  async previewCase(id: number, caseId: number, kind: string) {
    const r = await api.get(`/problems/${id}/testdata/${caseId}/preview/${kind}`)
    return r.data as string
  },
  async deleteCase(id: number, caseId: number) {
    const r = await api.delete(`/problems/${id}/testdata/${caseId}`)
    return r.data
  },
  async externalSources() {
    const r = await api.get('/admin/plugins/sources')
    return r.data as { problem_sources: string[]; submit_fetchers: string[] }
  },
  async caseScores(id: number) {
    const r = await api.get(`/problems/${id}/case-scores`)
    return r.data as { scores: Record<string, number>; total: number; cases: number }
  },
  async setCaseScores(id: number, scores: Record<string, number>) {
    const r = await api.put(`/problems/${id}/case-scores`, { scores })
    return r.data as { ok: boolean; total: number }
  },
}

export const Submissions = {
  async list(params: Record<string, unknown>) {
    const r = await api.get('/submissions', { params })
    return r.data as { total: number; items: Submission[] }
  },
  async get(id: number | string) {
    const r = await api.get(`/submissions/${id}`)
    return r.data as Submission
  },
  async create(payload: { problem_id: number; language: string; code: string; contest_id?: number }) {
    const r = await api.post('/submissions', payload)
    return r.data as Submission
  },
  async cancelContestSubmission(contestId: number | string, sid: number, cancelled: boolean) {
    const r = await api.post(`/contests/${contestId}/submissions/${sid}/cancel`, { cancelled })
    return r.data
  },
  async rejudgeContestSubmission(contestId: number | string, sid: number) {
    const r = await api.post(`/contests/${contestId}/submissions/${sid}/rejudge`)
    return r.data
  },
}

export const Contests = {
  async list() {
    const r = await api.get('/contests')
    return r.data as Contest[]
  },
  async get(id: number | string) {
    const r = await api.get(`/contests/${id}`)
    return r.data as {
      contest: Contest
      problems: ContestProblemItem[]
      notices?: ContestNotice[]
      is_judge?: boolean
    }
  },
  async create(payload: Partial<Contest>) {
    const r = await api.post('/contests', payload)
    return r.data as Contest
  },
  async setProblems(id: number, problemIds: number[]) {
    const r = await api.post(`/contests/${id}/problems`, { problem_ids: problemIds })
    return r.data
  },
  async standings(id: number | string) {
    const r = await api.get(`/contests/${id}/standings`)
    return r.data as { rows: StandingRow[] }
  },
  // ---- jury (creator/admin) ----
  async notices(id: number | string) {
    const r = await api.get(`/contests/${id}/notices`)
    return r.data as ContestNotice[]
  },
  async createNotice(id: number | string, content: string) {
    const r = await api.post(`/contests/${id}/notices`, { content })
    return r.data as ContestNotice
  },
  async deleteNotice(id: number | string, nid: number) {
    const r = await api.delete(`/contests/${id}/notices/${nid}`)
    return r.data
  },
  async flags(id: number | string) {
    const r = await api.get(`/contests/${id}/flags`)
    return r.data as ContestUserFlag[]
  },
  async participants(id: number | string) {
    const r = await api.get(`/contests/${id}/participants`)
    return r.data as UserBrief[]
  },
  async searchContestUsers(id: number | string, q: string) {
    const r = await api.get(`/contests/${id}/users/search`, { params: { q } })
    return r.data as UserBrief[]
  },
  async setUserFlags(id: number | string, userId: number, starred: boolean, cheated: boolean) {
    const r = await api.put(`/contests/${id}/users/${userId}/flags`, { starred, cheated })
    return r.data as ContestUserFlag
  },
  async setFreeze(id: number | string, frozen: boolean) {
    const r = await api.put(`/contests/${id}/freeze`, { frozen })
    return r.data
  },
  async setTime(id: number | string, startTime: string, endTime: string) {
    const r = await api.put(`/contests/${id}/time`, { start_time: startTime, end_time: endTime })
    return r.data
  },
  async rejudgeProblem(id: number | string, pid: number) {
    const r = await api.post(`/contests/${id}/problems/${pid}/rejudge`)
    return r.data as { requeued: number }
  },
  async cancelSubmission(id: number | string, sid: number, cancelled: boolean) {
    const r = await api.post(`/contests/${id}/submissions/${sid}/cancel`, { cancelled })
    return r.data
  },
  async cancelUserProblem(id: number | string, userId: number, pid: number, cancelled: boolean) {
    const r = await api.post(`/contests/${id}/users/${userId}/problems/${pid}/cancel`, { cancelled })
    return r.data
  },
  async reveal(id: number | string, count: number) {
    const r = await api.put(`/contests/${id}/reveal`, { count })
    return r.data
  },
  async register(id: number | string, teamName: string, teamType: string) {
    const r = await api.post(`/contests/${id}/register`, { team_name: teamName, team_type: teamType })
    return r.data as ContestRegistration
  },
  // 组队赛：队长以队为单位报名
  async registerTeam(id: number | string, teamId: number, teamType: string) {
    const r = await api.post(`/contests/${id}/register`, { team_id: teamId, team_type: teamType })
    return r.data
  },
  async myRegistration(id: number | string) {
    const r = await api.get(`/contests/${id}/my-registration`)
    return (r.data as { registration: ContestRegistration | null }).registration
  },
  async registrations(id: number | string) {
    const r = await api.get(`/contests/${id}/registrations`)
    return r.data as ContestRegistration[]
  },
  async editMyRegistration(id: number | string, teamName: string) {
    const r = await api.put(`/contests/${id}/my-registration`, { team_name: teamName })
    return r.data as ContestRegistration
  },
  async cancelMyRegistration(id: number | string) {
    const r = await api.delete(`/contests/${id}/my-registration`)
    return r.data
  },
  async adminEditRegistration(id: number | string, userId: number, teamName: string, teamType: string) {
    const r = await api.put(`/contests/${id}/registrations/${userId}`, { team_name: teamName, team_type: teamType })
    return r.data as ContestRegistration
  },
  async adminDeleteRegistration(id: number | string, userId: number) {
    const r = await api.delete(`/contests/${id}/registrations/${userId}`)
    return r.data
  },
}

export const Admin = {
  async users(q: string) {
    const r = await api.get('/admin/users', { params: { q } })
    return r.data as User[]
  },
  async importUsers(file: File) {
    const form = new FormData()
    form.append('file', file)
    const r = await api.post('/admin/users/import', form)
    return r.data as { created: number; rows: { username: string; password: string; err: string }[] }
  },
  async createInvite(payload: { max_uses: number; expires_in_hours: number }) {
    const r = await api.post('/admin/invite-codes', payload)
    return r.data as { code: string; max_uses: number }
  },
  async invites() {
    const r = await api.get('/admin/invite-codes')
    return r.data as { id: number; code: string; max_uses: number; used_count: number; expires_at: string | null }[]
  },
  async deleteInvite(id: number) {
    const r = await api.delete(`/admin/invite-codes/${id}`)
    return r.data
  },
  async createApiKey(name: string) {
    const r = await api.post('/admin/api-keys', { name })
    return r.data as { id: number; name: string; key: string; prefix: string }
  },
  async apiKeys() {
    const r = await api.get('/admin/api-keys')
    return r.data as { id: number; name: string; prefix: string; revoked: boolean; created_at: string; last_used: string | null }[]
  },
  async revokeApiKey(id: number) {
    const r = await api.post(`/admin/api-keys/${id}/revoke`)
    return r.data
  },
  async daemons() {
    const r = await api.get('/admin/daemons')
    return r.data as { daemons: { id: number; name: string; status: string; capacity: number; active_tasks: number; last_heartbeat: string | null }[]; queue_length: number }
  },
  async rejudge(id: number) {
    const r = await api.post(`/admin/rejudge/${id}`)
    return r.data
  },
  async updateUser(id: number, studentNo: string, nickname: string) {
    const r = await api.put(`/admin/users/${id}/profile`, { student_no: studentNo, nickname })
    return r.data
  },
  async resetPassword(id: number, password?: string) {
    const r = await api.post(`/admin/users/${id}/reset-password`, password ? { password } : {})
    return r.data as { password: string }
  },
  async setRole(id: number, role: string) {
    const r = await api.put(`/admin/users/${id}/role`, { role })
    return r.data
  },
  async setSuperRole(id: number, role: string) {
    const r = await api.put(`/admin/users/${id}/role/super`, { role })
    return r.data
  },
  async setBan(id: number, banned: boolean) {
    const r = await api.post(`/admin/users/${id}/ban`, { banned })
    return r.data
  },
}

export interface ExternalBinding {
  id: number
  user_id: number
  platform: string
  handle: string
  synced_at: string | null
  last_error?: string
  created_at: string
}

export interface ExternalReport {
  user_id: number
  platforms: string[]
  by_platform: { platform: string; submissions: number; ac: number; solved: number }[]
  activity: { date: string; submissions: number; ac: number }[]
  recent_ac: { platform: string; problem_id: string; problem_name: string; at: number; external_id: string }[]
  bindings: ExternalBinding[]
}

export interface ProblemMeta {
  source: string
  external_id: string
  url: string
  title: string
  statement_md: string
  time_limit_ms: number
  mem_limit_mb: number
  tags: string[]
  notes: string[]
}

// External — plugin-powered integrations: 题库爬取 (problem import from
// external OJs) and 刷题统计 (bound platform accounts + sync + reports).
export const External = {
  async platforms() {
    const r = await api.get('/external/bindings')
    return r.data as { bindings: ExternalBinding[]; platforms: string[] }
  },
  async bind(platform: string, handle: string) {
    const r = await api.put('/external/bindings', { platform, handle })
    return r.data as { binding: ExternalBinding; stored: number; error: string }
  },
  async unbind(platform: string) {
    const r = await api.delete(`/external/bindings/${platform}`)
    return r.data
  },
  async sync(platform: string) {
    const r = await api.post(`/external/bindings/${platform}/sync`)
    return r.data as { binding: ExternalBinding; stored: number; error: string }
  },
  async report(userId: number | string) {
    const r = await api.get(`/external/report/${userId}`)
    return r.data as ExternalReport
  },
  async previewProblem(source: string, externalId: string) {
    const r = await api.post('/external/problems/preview', { source, external_id: externalId })
    return r.data as { meta: ProblemMeta }
  },
  async importProblem(source: string, externalId: string, visibility?: string) {
    const r = await api.post('/external/problems/import', { source, external_id: externalId, visibility })
    return r.data as { problem: Problem; source: string; external_id: string; notes: string[] }
  },
}

export const Plugins = {
  async list() {
    const r = await api.get('/admin/plugins')
    return r.data as {
      problem_sources: string[]
      submit_fetchers: string[]
      event_hooks: string[]
      hook_targets: Record<string, string>
    }
  },
  async setHook(name: string, url: string) {
    const r = await api.put(`/admin/hooks/${name}`, { url })
    return r.data as { targets: Record<string, string> }
  },
  async deleteHook(name: string) {
    const r = await api.delete(`/admin/hooks/${name}`)
    return r.data as { targets: Record<string, string> }
  },
}

export const CaseScores = {
  async list(problemId: number) {
    const r = await api.get(`/problems/${problemId}/case-scores`)
    return r.data as { scores: Record<string, number>; total: number; cases: number }
  },
  async set(problemId: number, scores: Record<string, number>) {
    const r = await api.put(`/problems/${problemId}/case-scores`, { scores })
    return r.data as { ok: boolean; total: number }
  },
}

export function connectWS(topics: string[], onMessage: (topic: string, data: unknown) => void) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const token = encodeURIComponent(localStorage.getItem('oj_token') ?? '')
  // browsers can't set headers on WebSocket; the server validates ?token=
  const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws?token=${token}`)
  ws.onopen = () => {
    for (const t of topics) ws.send(JSON.stringify({ subscribe: t }))
  }
  ws.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data) as { topic: string; data: unknown }
      onMessage(msg.topic, msg.data)
    } catch {
      /* ignore malformed frames */
    }
  }
  return ws
}
