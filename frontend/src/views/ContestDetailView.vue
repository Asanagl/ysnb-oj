<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Contests, Teams, connectWS, errMsg, type StandingRow } from '../api/client'
import { renderStatement } from '../utils/markdown'
import { useAuthStore } from '../stores/auth'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'
import ContestSubmissions from '../components/ContestSubmissions.vue'
import ScoreBoard from '../components/ScoreBoard.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { NumberInput } from '@/components/ui/number-input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Checkbox } from '@/components/ui/checkbox'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert } from '@/components/ui/alert'
import { Separator } from '@/components/ui/separator'

const auth = useAuthStore()

async function registerContest() {
  try {
    myReg.value = await Contests.register(cid.value, regForm.value.team_name, regForm.value.team_type)
    toast.success(regForm.value.team_type === 'starred' ? '已报名（打星队）' : '已报名（正式队）')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

// 组队赛：队长以队为单位报名
const isTeamMode = computed(() => (data.value?.contest as { team_mode?: boolean } | undefined)?.team_mode === true)
const regTeamId = ref<number>()
const regTeamSelect = computed({
  get: () => regTeamId.value,
  set: (v: string | number | undefined) => {
    regTeamId.value = v == null ? undefined : Number(v)
  },
})
const myCaptainedTeams = ref<{ id: number; name: string; member_count: number }[]>([])

async function registerTeam() {
  if (!regTeamId.value) {
    toast.warning('先选择要报名的小组')
    return
  }
  try {
    await Contests.registerTeam(cid.value, regTeamId.value, regForm.value.team_type)
    toast.success('已以队为单位报名，全队成员共同参赛')
    location.reload()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function saveRegName() {
  if (!myReg.value) return
  try {
    myReg.value = await Contests.editMyRegistration(cid.value, myReg.value.team_name)
    toast.success('队伍名已更新')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function cancelReg() {
  if (!(await confirmDialog({
    title: '确定退赛？',
    description: '提交记录将保留但报名作废',
    danger: true,
  }))) return
  try {
    await Contests.cancelMyRegistration(cid.value)
    myReg.value = null
    toast.success('已取消报名')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function saveReveal() {
  try {
    await Contests.reveal(cid.value, revealCount.value)
    toast.success(`已揭示最后 ${revealCount.value} 名`)
    await loadStandings()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const route = useRoute()
const router = useRouter()
const data = ref<Awaited<ReturnType<typeof Contests.get>> | null>(null)
const rows = ref<StandingRow[]>([])
const notices = ref<Awaited<ReturnType<typeof Contests.notices>>>([])
const tab = ref('problems')
let ws: ReturnType<typeof connectWS> | null = null

const cid = computed(() => route.params.id as string)
async function loadStandings() {
  rows.value = (await Contests.standings(cid.value)).rows
}

const isJudge = computed(() => data.value?.is_judge === true)
// immediate watch 会在 setup 期间立即求值 isTeamMode，必须排在 data 声明之后（TDZ）
watch(isTeamMode, async (v) => {
  if (!v || !auth.logged) return
  try {
    const all = await Teams.all()
    myCaptainedTeams.value = all.filter((t) => t.role === 'captain')
  } catch { /* non-fatal */ }
}, { immediate: true })
const boardMode = computed<'acm' | 'ioi'>(() => (data.value?.contest.mode === 'ioi' ? 'ioi' : 'acm'))
const contestFrozenNow = computed(() => {
  if (!data.value) return false
  const c = data.value.contest
  const now = Date.now()
  if (!c.freeze_enabled || now > new Date(c.end_time).getTime()) return false
  if (c.manual_frozen) return true
  return c.freeze_time != null && now > new Date(c.freeze_time).getTime()
})
const noticeText = ref('')
const timeForm = ref({ start_time: '', end_time: '' })
const frozen = ref(false)
const flagForm = ref({ user_id: null as number | null, starred: false, cheated: false })
// Select 的 model 不含 null，这里做一层 null ↔ undefined 的桥接
const flagUserSelect = computed({
  get: () => flagForm.value.user_id ?? undefined,
  set: (v: string | number | undefined) => {
    flagForm.value.user_id = v == null ? null : Number(v)
  },
})
// 用户标记下拉数据源：默认参赛者（有提交者），搜索时合并全量用户
const participants = ref<Awaited<ReturnType<typeof Contests.participants>>>([])
const searchResults = ref<Awaited<ReturnType<typeof Contests.searchContestUsers>>>([])
const flagged = ref<Awaited<ReturnType<typeof Contests.flags>>>([])
const registrations = ref<Awaited<ReturnType<typeof Contests.registrations>>>([])
const myReg = ref<Awaited<ReturnType<typeof Contests.myRegistration>>>(null)
const regForm = ref({ team_name: '', team_type: 'official' })
const revealCount = ref(0)

// reka Select 不允许重复 value，搜索结果里已在参赛者分组的用户不再重复展示
const searchOnlyResults = computed(() => {
  const ids = new Set(participants.value.map((u) => u.id))
  return searchResults.value.filter((u) => !ids.has(u.id))
})

async function loadJuryData() {
  if (!isJudge.value) return
  participants.value = await Contests.participants(cid.value)
  flagged.value = await Contests.flags(cid.value)
  registrations.value = await Contests.registrations(cid.value)
}

// remote search: empty query keeps participants; typing searches all users
async function searchUsers(query: string) {
  if (!query.trim()) {
    searchResults.value = []
    return
  }
  try {
    searchResults.value = await Contests.searchContestUsers(cid.value, query.trim())
  } catch {
    searchResults.value = []
  }
}

function onFlagSearchInput(e: Event) {
  searchUsers((e.target as HTMLInputElement).value)
}

const userLabel = (u: { id: number; username: string; nickname: string }) =>
  `${u.nickname || u.username} (@${u.username} · #${u.id})`

// prefill checkboxes from existing flags when a user is picked
watch(
  () => flagForm.value.user_id,
  (id) => {
    const f = flagged.value.find((x) => x.user_id === id)
    flagForm.value.starred = !!f?.starred
    flagForm.value.cheated = !!f?.cheated
  },
)

async function applyFlags() {
  if (!flagForm.value.user_id) {
    toast.warning('先在下拉框选择选手')
    return
  }
  try {
    await Contests.setUserFlags(
      cid.value,
      flagForm.value.user_id,
      flagForm.value.starred,
      flagForm.value.cheated,
    )
    toast.success('标记已保存，榜单已更新')
    flagged.value = await Contests.flags(cid.value)
    await loadStandings()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function loadRegistrations() {
  registrations.value = await Contests.registrations(cid.value)
}

async function changeRegType(userId: number, teamName: string, teamType: string) {
  try {
    await Contests.adminEditRegistration(cid.value, userId, teamName, teamType)
    toast.success('类型已更新')
    await loadRegistrations()
    await loadStandings()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function removeRegistration(userId: number) {
  if (!(await confirmDialog({ title: '删除该报名？', danger: true }))) return
  try {
    await Contests.adminDeleteRegistration(cid.value, userId)
    toast.success('报名已删除')
    await loadRegistrations()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function clearFlags(userId: number) {
  try {
    await Contests.setUserFlags(cid.value, userId, false, false)
    toast.success('已撤销标记')
    flagged.value = await Contests.flags(cid.value)
    await loadStandings()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

onMounted(async () => {
  data.value = await Contests.get(cid.value)
  notices.value = await Contests.notices(cid.value)
  frozen.value = !!data.value.contest.manual_frozen
  revealCount.value = data.value.contest.reveal_count ?? 0
  timeForm.value.start_time = data.value.contest.start_time.slice(0, 16)
  timeForm.value.end_time = data.value.contest.end_time.slice(0, 16)
  if (auth.logged) {
    try {
      myReg.value = await Contests.myRegistration(cid.value)
    } catch { /* visitor without token */ }
  }
  await loadStandings()
  await loadJuryData()
  ws = connectWS([`contest:${route.params.id}`], () => loadStandings())
})
onBeforeUnmount(() => ws?.close())

const remaining = computed(() => {
  if (!data.value) return ''
  const ms = new Date(data.value.contest.end_time).getTime() - Date.now()
  if (ms <= 0) return '已结束 · 补题模式'
  const h = Math.floor(ms / 3600000)
  const m = Math.floor((ms % 3600000) / 60000)
  return `剩余 ${h} 小时 ${m} 分钟`
})

async function publishNotice() {
  if (!noticeText.value.trim()) return
  try {
    await Contests.createNotice(cid.value, noticeText.value.trim())
    noticeText.value = ''
    notices.value = await Contests.notices(cid.value)
    toast.success('公告已发布')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function removeNotice(id: number) {
  try {
    await Contests.deleteNotice(cid.value, id)
    notices.value = await Contests.notices(cid.value)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function saveFreeze() {
  try {
    await Contests.setFreeze(cid.value, frozen.value)
    toast.success(frozen.value ? '榜单已手动冻结' : '榜单已解除冻结')
    data.value = await Contests.get(cid.value)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function saveTime() {
  try {
    await Contests.setTime(
      cid.value,
      new Date(timeForm.value.start_time).toISOString(),
      new Date(timeForm.value.end_time).toISOString(),
    )
    toast.success('比赛时间已调整')
    data.value = await Contests.get(cid.value)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function rejudge(pid: number) {
  if (!(await confirmDialog({
    title: '重测本题？',
    description: '本题的全部提交将重新排队评测',
    danger: true,
  }))) return
  try {
    const r = (await Contests.rejudgeProblem(cid.value, pid)) as unknown as {
      requeued?: number
    }
    toast.success(`已重测 ${r.requeued ?? 0} 条提交`)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

function exportCSV() {
  // why location: the endpoint responds with Content-Disposition attachment.
  window.location.href = `/api/v1/contests/${cid.value}/standings.csv`
}

function openProjection() {
  // why window here: the projection screen must open as a separate tab so
  // the operator keeps the management page.
  window.open(`/contests/${route.params.id}/board`, '_blank')
}

</script>

<template>
  <div v-if="data">
    <Card>
      <CardContent class="p-4">
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="m-0 text-xl font-semibold">{{ data.contest.title }}</h2>
          <span class="text-muted-foreground">{{ remaining }}</span>
          <Badge v-if="data.contest.manual_frozen" variant="tle">榜单已冻结</Badge>
          <Button size="sm" variant="outline" @click="exportCSV">导出榜单 CSV</Button>
          <Button size="sm" variant="secondary" @click="openProjection">投屏模式</Button>
        </div>
        <div class="mt-3" v-html="renderStatement(data.contest.description)" />
      </CardContent>
    </Card>

    <Card v-if="notices.length || isJudge" class="mt-3">
      <CardContent class="p-4">
        <h4 class="mb-2 mt-0 text-base font-semibold">公告</h4>
        <div v-for="n in notices" :key="n.id" class="mb-1.5 flex items-center gap-2">
          <span class="flex-1">{{ n.content }}</span>
          <span class="whitespace-nowrap text-xs text-muted-foreground">
            {{ new Date(n.created_at).toLocaleString() }}
          </span>
          <Button v-if="isJudge" size="sm" variant="ghost" @click="removeNotice(n.id)">删除</Button>
        </div>
        <div v-if="isJudge" class="mt-2 flex gap-2">
          <Input v-model="noticeText" placeholder="发布新公告（如：B 题数据已更新并重测）" />
          <Button @click="publishNotice">发布</Button>
        </div>
      </CardContent>
    </Card>

    <Card v-if="auth.logged && !myReg && !isJudge && !isTeamMode" class="mt-3">
      <CardContent class="p-4">
        <h4 class="mb-2 mt-0 text-base font-semibold">报名参赛</h4>
        <div class="flex flex-wrap items-center gap-3">
          <Input v-model="regForm.team_name" placeholder="队伍名（可选）" class="w-56" />
          <Select v-model="regForm.team_type">
            <SelectTrigger class="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="official">正式队伍</SelectItem>
              <SelectItem value="starred">打星队（不占名次）</SelectItem>
            </SelectContent>
          </Select>
          <Button @click="registerContest">报名</Button>
        </div>
      </CardContent>
    </Card>

    <!-- 组队赛报名：队长选择自己的小组，以队为单位报名 -->
    <Card v-if="auth.logged && !myReg && !isJudge && isTeamMode" class="mt-3">
      <CardContent class="p-4">
        <h4 class="mb-2 mt-0 text-base font-semibold">组队报名（ICPC 三人一队）</h4>
        <div class="flex flex-wrap items-center gap-3">
          <Select v-model="regTeamSelect">
            <SelectTrigger class="w-60">
              <SelectValue placeholder="选择我的小组" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="t in myCaptainedTeams" :key="t.id" :value="t.id">
                {{ t.name }}（{{ t.member_count }} 人）
              </SelectItem>
            </SelectContent>
          </Select>
          <Select v-model="regForm.team_type">
            <SelectTrigger class="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="official">正式队伍</SelectItem>
              <SelectItem value="starred">打星队（不占名次）</SelectItem>
            </SelectContent>
          </Select>
          <Button @click="registerTeam">以队报名</Button>
        </div>
        <p v-if="myCaptainedTeams.length === 0" class="mb-0 mt-2 text-xs text-muted-foreground">
          你还没有担任队长的小组，请先到「小组」页创建或管理队伍
        </p>
      </CardContent>
    </Card>
    <Card v-if="myReg" class="mt-3">
      <CardContent class="p-4">
        <div class="flex flex-wrap items-center gap-3">
          <Badge :variant="myReg.team_type === 'starred' ? 'tle' : 'ac'">
            {{ myReg.team_type === 'starred' ? '★ 打星队' : '正式队' }}
          </Badge>
          <Input
            v-model="myReg.team_name"
            placeholder="队伍名"
            class="w-52"
            @change="saveRegName"
          />
          <span class="text-xs text-muted-foreground">改队名后回车保存</span>
          <span class="flex-1" />
          <Button variant="ghost" size="sm" class="text-destructive" @click="cancelReg">取消报名</Button>
        </div>
      </CardContent>
    </Card>

    <Tabs v-model="tab" class="mt-3">
      <TabsList>
        <TabsTrigger value="problems">题目</TabsTrigger>
        <TabsTrigger value="subs">提交记录</TabsTrigger>
        <TabsTrigger value="standings">榜单</TabsTrigger>
        <TabsTrigger v-if="isJudge" value="jury">裁判台</TabsTrigger>
      </TabsList>

      <TabsContent value="problems">
        <Alert
          variant="info"
          title="每道题都有独立副本——只属于本场比赛，从这里的入口提交才会计入榜单"
          class="mb-3"
        />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-20" />
              <TableHead>题目</TableHead>
              <TableHead class="w-28">时限</TableHead>
              <TableHead class="w-28">内存</TableHead>
              <TableHead v-if="isJudge" class="w-32">裁判</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="row in data.problems"
              :key="row.id"
              class="cursor-pointer"
              @click="router.push(`/contests/${route.params.id}/problems/${row.id}`)"
            >
              <TableCell>{{ row.label }}</TableCell>
              <TableCell>{{ row.title }}</TableCell>
              <TableCell>{{ row.time_limit_ms }} ms</TableCell>
              <TableCell>{{ row.mem_limit_mb }} MB</TableCell>
              <TableCell v-if="isJudge">
                <Button size="sm" @click.stop="rejudge(row.id)">重测本题</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TabsContent>

      <TabsContent value="subs">
        <ContestSubmissions :cid="Number(route.params.id)" :is-judge="isJudge" :frozen="contestFrozenNow" />
      </TabsContent>

      <TabsContent value="standings">
        <ScoreBoard :rows="rows" :problems="data.problems" :mode="boardMode" />
      </TabsContent>

      <TabsContent v-if="isJudge" value="jury">
        <div class="grid gap-4 md:grid-cols-2">
          <Card>
            <CardContent class="p-4">
              <h4 class="mb-2 mt-0 text-base font-semibold">比赛时间</h4>
              <Input v-model="timeForm.start_time" placeholder="开始 YYYY-MM-DDTHH:mm" class="mb-2" />
              <Input v-model="timeForm.end_time" placeholder="结束 YYYY-MM-DDTHH:mm" class="mb-2" />
              <Button @click="saveTime">保存时间</Button>
              <Separator class="my-4" />
              <h4 class="mb-2 mt-0 text-base font-semibold">手动封榜</h4>
              <div class="flex items-center gap-2">
                <Switch v-model="frozen" @update:model-value="saveFreeze" />
                <span class="text-sm text-muted-foreground">{{ frozen ? '冻结' : '解封' }}</span>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent class="p-4">
              <h4 class="mb-1 mt-0 text-base font-semibold">滚榜揭示控制台</h4>
              <p class="text-xs text-muted-foreground">
                按真实最终榜从最后一名向前逐队解除遮罩；已揭示队伍的真实成绩即刻上榜。
              </p>
              <div class="flex items-center gap-3">
                <NumberInput v-model="revealCount" :min="0" :max="500" class="w-36" />
                <Button variant="secondary" @click="saveReveal">应用揭示</Button>
              </div>
              <Separator class="my-4" />
              <h4 class="mb-2 mt-0 text-base font-semibold">报名管理</h4>
              <Table class="mb-2">
                <TableHeader>
                  <TableRow>
                    <TableHead>用户</TableHead>
                    <TableHead class="w-32">队伍名</TableHead>
                    <TableHead class="w-24">类型</TableHead>
                    <TableHead class="w-44">改类型</TableHead>
                    <TableHead class="w-20">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="!registrations.length">
                    <TableCell :colspan="5" class="text-center text-muted-foreground">暂无数据</TableCell>
                  </TableRow>
                  <TableRow v-for="row in registrations" :key="row.user_id">
                    <TableCell>{{ row.username || row.user_id }}</TableCell>
                    <TableCell>{{ row.team_name }}</TableCell>
                    <TableCell>
                      <Badge :variant="row.team_type === 'starred' ? 'tle' : 'ac'">
                        {{ row.team_type === 'starred' ? '★ 打星' : '正式' }}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Select
                        :model-value="row.team_type"
                        @update:model-value="(v: string | number | undefined) => changeRegType(row.user_id, row.team_name, String(v))"
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="official">正式队伍</SelectItem>
                          <SelectItem value="starred">★ 打星队</SelectItem>
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      <Button
                        size="sm"
                        variant="ghost"
                        class="text-destructive"
                        @click="removeRegistration(row.user_id)"
                      >
                        删除
                      </Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
              <h4 class="mb-2 mt-0 text-base font-semibold">用户标记（打星 / 作弊）</h4>
              <Input
                placeholder="输入用户名/昵称/ID 搜索全量用户"
                class="mb-2"
                @input="onFlagSearchInput"
              />
              <Select v-model="flagUserSelect">
                <SelectTrigger class="mb-2.5 w-full">
                  <SelectValue placeholder="选择参赛选手，或输入用户名/昵称/ID 搜索全量用户" />
                </SelectTrigger>
                <SelectContent>
                  <div class="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                    参赛选手（本场有提交）
                  </div>
                  <SelectItem v-for="u in participants" :key="'p' + u.id" :value="u.id">
                    {{ userLabel(u) }}
                  </SelectItem>
                  <template v-if="searchOnlyResults.length">
                    <div class="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                      全量搜索结果
                    </div>
                    <SelectItem v-for="u in searchOnlyResults" :key="'s' + u.id" :value="u.id">
                      {{ userLabel(u) }}
                    </SelectItem>
                  </template>
                </SelectContent>
              </Select>
              <div class="mb-2 flex gap-4">
                <label class="flex cursor-pointer items-center gap-2 text-sm">
                  <Checkbox v-model="flagForm.starred" />
                  打星（不占名次）
                </label>
                <label class="flex cursor-pointer items-center gap-2 text-sm">
                  <Checkbox v-model="flagForm.cheated" />
                  作弊（出榜标红）
                </label>
              </div>
              <Button @click="applyFlags">保存标记</Button>
              <Separator class="my-4" />
              <h4 class="mb-2 mt-0 text-base font-semibold">已标记用户</h4>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>用户</TableHead>
                    <TableHead class="w-24">标记</TableHead>
                    <TableHead class="w-20">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="!flagged.length">
                    <TableCell :colspan="3" class="text-center text-muted-foreground">暂无数据</TableCell>
                  </TableRow>
                  <TableRow v-for="row in flagged" :key="row.user_id">
                    <TableCell>{{ row.username || row.user_id }}</TableCell>
                    <TableCell>
                      <Badge v-if="row.cheated" variant="destructive">作弊</Badge>
                      <Badge v-else-if="row.starred" variant="tle">★</Badge>
                    </TableCell>
                    <TableCell>
                      <Button size="sm" variant="ghost" @click="clearFlags(row.user_id)">撤销</Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
              <p class="mb-0 mt-2 text-xs text-muted-foreground">
                重测入口在「题目」tab 的裁判列；取消成绩/单条重判在「提交记录」tab。
              </p>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
