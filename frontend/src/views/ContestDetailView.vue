<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Contests, Teams, connectWS, errMsg, type StandingRow } from '../api/client'
import { renderStatement } from '../utils/markdown'
import { useAuthStore } from '../stores/auth'
import ContestSubmissions from '../components/ContestSubmissions.vue'
import ScoreBoard from '../components/ScoreBoard.vue'

const auth = useAuthStore()

async function registerContest() {
  try {
    myReg.value = await Contests.register(cid.value, regForm.value.team_name, regForm.value.team_type)
    ElMessage.success(regForm.value.team_type === 'starred' ? '已报名（打星队）' : '已报名（正式队）')
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

// 组队赛：队长以队为单位报名
const isTeamMode = computed(() => (data.value?.contest as { team_mode?: boolean } | undefined)?.team_mode === true)
const regTeamId = ref<number>()
const myCaptainedTeams = ref<{ id: number; name: string; member_count: number }[]>([])
watch(isTeamMode, async (v) => {
  if (!v || !auth.logged) return
  try {
    const all = await Teams.all()
    myCaptainedTeams.value = all.filter((t) => t.role === 'captain')
  } catch { /* non-fatal */ }
}, { immediate: true })

async function registerTeam() {
  if (!regTeamId.value) {
    ElMessage.warning('先选择要报名的小组')
    return
  }
  try {
    await Contests.registerTeam(cid.value, regTeamId.value, regForm.value.team_type)
    ElMessage.success('已以队为单位报名，全队成员共同参赛')
    location.reload()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function saveRegName() {
  if (!myReg.value) return
  try {
    myReg.value = await Contests.editMyRegistration(cid.value, myReg.value.team_name)
    ElMessage.success('队伍名已更新')
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function cancelReg() {
  try {
    await Contests.cancelMyRegistration(cid.value)
    myReg.value = null
    ElMessage.success('已取消报名')
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function saveReveal() {
  try {
    await Contests.reveal(cid.value, revealCount.value)
    ElMessage.success(`已揭示最后 ${revealCount.value} 名`)
    await loadStandings()
  } catch (e) {
    ElMessage.error(errMsg(e))
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
// 用户标记下拉数据源：默认参赛者（有提交者），搜索时合并全量用户
const participants = ref<Awaited<ReturnType<typeof Contests.participants>>>([])
const searchResults = ref<Awaited<ReturnType<typeof Contests.searchContestUsers>>>([])
const flagged = ref<Awaited<ReturnType<typeof Contests.flags>>>([])
const registrations = ref<Awaited<ReturnType<typeof Contests.registrations>>>([])
const myReg = ref<Awaited<ReturnType<typeof Contests.myRegistration>>>(null)
const regForm = ref({ team_name: '', team_type: 'official' })
const revealCount = ref(0)

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
    ElMessage.warning('先在下拉框选择选手')
    return
  }
  try {
    await Contests.setUserFlags(
      cid.value,
      flagForm.value.user_id,
      flagForm.value.starred,
      flagForm.value.cheated,
    )
    ElMessage.success('标记已保存，榜单已更新')
    flagged.value = await Contests.flags(cid.value)
    await loadStandings()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function loadRegistrations() {
  registrations.value = await Contests.registrations(cid.value)
}

async function changeRegType(userId: number, teamName: string, teamType: string) {
  try {
    await Contests.adminEditRegistration(cid.value, userId, teamName, teamType)
    ElMessage.success('类型已更新')
    await loadRegistrations()
    await loadStandings()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function removeRegistration(userId: number) {
  try {
    await Contests.adminDeleteRegistration(cid.value, userId)
    ElMessage.success('报名已删除')
    await loadRegistrations()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function clearFlags(userId: number) {
  try {
    await Contests.setUserFlags(cid.value, userId, false, false)
    ElMessage.success('已撤销标记')
    flagged.value = await Contests.flags(cid.value)
    await loadStandings()
  } catch (e) {
    ElMessage.error(errMsg(e))
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
    ElMessage.success('公告已发布')
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function removeNotice(id: number) {
  try {
    await Contests.deleteNotice(cid.value, id)
    notices.value = await Contests.notices(cid.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function saveFreeze() {
  try {
    await Contests.setFreeze(cid.value, frozen.value)
    ElMessage.success(frozen.value ? '榜单已手动冻结' : '榜单已解除冻结')
    data.value = await Contests.get(cid.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function saveTime() {
  try {
    await Contests.setTime(
      cid.value,
      new Date(timeForm.value.start_time).toISOString(),
      new Date(timeForm.value.end_time).toISOString(),
    )
    ElMessage.success('比赛时间已调整')
    data.value = await Contests.get(cid.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function rejudge(pid: number) {
  try {
    const r = (await Contests.rejudgeProblem(cid.value, pid)) as unknown as {
      requeued?: number
    }
    ElMessage.success(`已重测 ${r.requeued ?? 0} 条提交`)
  } catch (e) {
    ElMessage.error(errMsg(e))
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
    <el-card>
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
        <h2 style="margin: 0">{{ data.contest.title }}</h2>
        <span style="color: #909399">{{ remaining }}</span>
        <el-tag v-if="data.contest.manual_frozen" type="warning">榜单已冻结</el-tag>
        <el-button size="small" @click="exportCSV">导出榜单 CSV</el-button>
        <el-button
          size="small"
          type="warning"
          @click="openProjection"
        >
          投屏模式
        </el-button>
      </div>
      <div v-html="renderStatement(data.contest.description)" />
    </el-card>

    <el-card v-if="notices.length || isJudge" style="margin-top: 12px">
      <h4 style="margin: 0 0 8px">公告</h4>
      <div v-for="n in notices" :key="n.id" style="display: flex; gap: 8px; margin-bottom: 6px">
        <span style="flex: 1">{{ n.content }}</span>
        <span style="color: #c0c4cc; font-size: 12px; white-space: nowrap">
          {{ new Date(n.created_at).toLocaleString() }}
        </span>
        <el-button v-if="isJudge" size="small" text @click="removeNotice(n.id)">删除</el-button>
      </div>
      <div v-if="isJudge" style="display: flex; gap: 8px; margin-top: 8px">
        <el-input v-model="noticeText" placeholder="发布新公告（如：B 题数据已更新并重测）" />
        <el-button type="primary" @click="publishNotice">发布</el-button>
      </div>
    </el-card>

    <el-card v-if="auth.logged && !myReg && !isJudge && !isTeamMode" style="margin-top: 12px">
      <h4 style="margin-top: 0">报名参赛</h4>
      <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap">
        <el-input
          v-model="regForm.team_name"
          placeholder="队伍名（可选）"
          style="width: 220px"
        />
        <el-select v-model="regForm.team_type" style="width: 160px">
          <el-option label="正式队伍" value="official" />
          <el-option label="打星队（不占名次）" value="starred" />
        </el-select>
        <el-button type="primary" @click="registerContest">报名</el-button>
      </div>
    </el-card>

    <!-- 组队赛报名：队长选择自己的小组，以队为单位报名 -->
    <el-card v-if="auth.logged && !myReg && !isJudge && isTeamMode" style="margin-top: 12px">
      <h4 style="margin-top: 0">组队报名（ICPC 三人一队）</h4>
      <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap">
        <el-select v-model="regTeamId" placeholder="选择我的小组" style="width: 240px">
          <el-option v-for="t in myCaptainedTeams" :key="t.id" :label="`${t.name}（${t.member_count} 人）`" :value="t.id" />
        </el-select>
        <el-select v-model="regForm.team_type" style="width: 160px">
          <el-option label="正式队伍" value="official" />
          <el-option label="打星队（不占名次）" value="starred" />
        </el-select>
        <el-button type="primary" @click="registerTeam">以队报名</el-button>
      </div>
      <p v-if="myCaptainedTeams.length === 0" style="color: #909399; font-size: 12px; margin: 8px 0 0">
        你还没有担任队长的小组，请先到「小组」页创建或管理队伍
      </p>
    </el-card>
    <el-card v-if="myReg" style="margin-top: 12px">
      <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap">
        <el-tag :type="myReg.team_type === 'starred' ? 'warning' : 'success'">
          {{ myReg.team_type === 'starred' ? '★ 打星队' : '正式队' }}
        </el-tag>
        <el-input
          v-model="myReg.team_name"
          placeholder="队伍名"
          style="width: 200px"
          @change="saveRegName"
        />
        <span style="color: #909399; font-size: 12px">改队名后回车保存</span>
        <span style="flex: 1" />
        <el-popconfirm title="确定退赛？提交记录将保留但报名作废" @confirm="cancelReg">
          <template #reference>
            <el-button type="danger" text>取消报名</el-button>
          </template>
        </el-popconfirm>
      </div>
    </el-card>

    <el-tabs v-model="tab" style="margin-top: 12px">
      <el-tab-pane label="题目" name="problems">
        <el-alert
          title="每道题都有独立副本——只属于本场比赛，从这里的入口提交才会计入榜单"
          type="info"
          :closable="false"
          style="margin-bottom: 10px"
        />
        <el-table
          :data="data.problems"
          @row-click="(row: { id: number }) => router.push(`/contests/${route.params.id}/problems/${row.id}`)"
        >
          <el-table-column label="" prop="label" width="80" />
          <el-table-column label="题目" prop="title" />
          <el-table-column label="时限" width="110">
            <template #default="{ row }">{{ row.time_limit_ms }} ms</template>
          </el-table-column>
          <el-table-column label="内存" width="110">
            <template #default="{ row }">{{ row.mem_limit_mb }} MB</template>
          </el-table-column>
          <el-table-column v-if="isJudge" label="裁判" width="130">
            <template #default="{ row }">
              <el-button size="small" @click="rejudge(row.id)">重测本题</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="提交记录" name="subs">
        <ContestSubmissions :cid="Number(route.params.id)" :is-judge="isJudge" :frozen="contestFrozenNow" />
      </el-tab-pane>

      <el-tab-pane label="榜单" name="standings">
        <ScoreBoard :rows="rows" :problems="data.problems" />
      </el-tab-pane>

      <el-tab-pane v-if="isJudge" label="裁判台" name="jury">
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-card>
              <h4>比赛时间</h4>
              <el-input v-model="timeForm.start_time" placeholder="开始 YYYY-MM-DDTHH:mm" style="margin-bottom: 8px" />
              <el-input v-model="timeForm.end_time" placeholder="结束 YYYY-MM-DDTHH:mm" style="margin-bottom: 8px" />
              <el-button type="primary" @click="saveTime">保存时间</el-button>
              <el-divider />
              <h4>手动封榜</h4>
              <el-switch v-model="frozen" active-text="冻结" inactive-text="解封" @change="saveFreeze" />
            </el-card>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-card>
              <h4>滚榜揭示控制台</h4>
              <p style="color: #909399; font-size: 12px">
                按真实最终榜从最后一名向前逐队解除遮罩；已揭示队伍的真实成绩即刻上榜。
              </p>
              <div style="display: flex; gap: 12px; align-items: center">
                <el-input-number v-model="revealCount" :min="0" :max="500" />
                <el-button type="warning" @click="saveReveal">应用揭示</el-button>
              </div>
              <el-divider />
              <h4>报名管理</h4>
              <el-table :data="registrations" size="small" style="margin-bottom: 8px">
                <el-table-column label="用户" min-width="120">
                  <template #default="{ row }">{{ row.username || row.user_id }}</template>
                </el-table-column>
                <el-table-column label="队伍名" prop="team_name" width="130" />
                <el-table-column label="类型" width="100">
                  <template #default="{ row }">
                    <el-tag :type="row.team_type === 'starred' ? 'warning' : 'success'" size="small">
                      {{ row.team_type === 'starred' ? '★ 打星' : '正式' }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="改类型" width="200">
                  <template #default="{ row }">
                    <el-select
                      :model-value="row.team_type"
                      size="small"
                      @update:model-value="(v: string) => changeRegType(row.user_id, row.team_name, v)"
                    >
                      <el-option label="正式队伍" value="official" />
                      <el-option label="★ 打星队" value="starred" />
                    </el-select>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="90">
                  <template #default="{ row }">
                    <el-popconfirm title="删除该报名？" @confirm="removeRegistration(row.user_id)">
                      <template #reference>
                        <el-button size="small" type="danger" text>删除</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </el-table-column>
              </el-table>
              <h4>用户标记（打星 / 作弊）</h4>
              <el-select
                v-model="flagForm.user_id"
                filterable
                remote
                :remote-method="searchUsers"
                :loading="false"
                placeholder="选择参赛选手，或输入用户名/昵称/ID 搜索全量用户"
                style="width: 100%; margin-bottom: 10px"
              >
                <el-option-group label="参赛选手（本场有提交）">
                  <el-option
                    v-for="u in participants"
                    :key="'p' + u.id"
                    :label="userLabel(u)"
                    :value="u.id"
                  />
                </el-option-group>
                <el-option-group v-if="searchResults.length" label="全量搜索结果">
                  <el-option
                    v-for="u in searchResults"
                    :key="'s' + u.id"
                    :label="userLabel(u)"
                    :value="u.id"
                  />
                </el-option-group>
              </el-select>
              <div style="display: flex; gap: 16px; margin-bottom: 8px">
                <el-checkbox v-model="flagForm.starred" label="打星（不占名次）" />
                <el-checkbox v-model="flagForm.cheated" label="作弊（出榜标红）" />
              </div>
              <el-button type="primary" @click="applyFlags">保存标记</el-button>
              <el-divider />
              <h4>已标记用户</h4>
              <el-table :data="flagged" size="small">
                <el-table-column label="用户" min-width="130">
                  <template #default="{ row }">{{ row.username || row.user_id }}</template>
                </el-table-column>
                <el-table-column label="标记" width="90">
                  <template #default="{ row }">
                    <el-tag v-if="row.cheated" type="danger" size="small">作弊</el-tag>
                    <el-tag v-else-if="row.starred" type="warning" size="small">★</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="90">
                  <template #default="{ row }">
                    <el-button size="small" text @click="clearFlags(row.user_id)">撤销</el-button>
                  </template>
                </el-table-column>
              </el-table>
              <p style="color: #909399; font-size: 12px; margin: 8px 0 0">
                重测入口在「题目」tab 的裁判列；取消成绩/单条重判在「提交记录」tab。
              </p>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

