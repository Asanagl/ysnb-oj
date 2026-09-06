<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Users, errMsg, External, type ExternalBinding, type ExternalReport, type UserProfile } from '../api/client'
import { useAuthStore } from '../stores/auth'
import StatusTag from '../components/StatusTag.vue'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Stat } from '@/components/ui/stat'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cssVar, useChart } from '@/composables/useChart'
import { confirmDialog } from '@/lib/confirm'
import { toast } from '@/lib/toast'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const profile = ref<UserProfile | null>(null)
// heatmap platform selection: multi-select of bound external platforms
// merged into 本站 counts (presentation-only toggle; persisted locally).
const heatPlatforms = ref<string[]>([])

// own profile at /profile, public page at /users/:id
const userId = computed(() => {
  if (route.path === '/profile') return auth.user?.id
  return Number(route.params.id)
})

const heatRef = ref<HTMLDivElement>()
const trendRef = ref<HTMLDivElement>()
const pieRef = ref<HTMLDivElement>()
const tagRef = ref<HTMLDivElement>()
const { chartErrors, mount, disposeAll, resizeAll, watchTheme } = useChart()

function drawAll() {
  if (!profile.value) return
  disposeAll()

  const muted = cssVar('--muted-foreground')

  // heatmap merges 本站 activity with the selected external platforms
  // (heatPlatforms, persisted in localStorage as oj_heat_platforms).
  const extPlatforms = Object.keys(profile.value.platforms?.[0]?.by_platform ?? {})
  const chosen = new Set(heatPlatforms.value.filter((p) => extPlatforms.includes(p)))

  const merged = new Map<string, { submissions: number; ac: number }>()
  for (const d of profile.value.activity) {
    merged.set(d.date, { submissions: d.submissions, ac: d.ac })
  }
  for (const d of profile.value.platforms ?? []) {
    for (const [plat, st] of Object.entries(d.by_platform ?? {})) {
      if (!chosen.has(plat)) continue
      const cur = merged.get(d.date) ?? { submissions: 0, ac: 0 }
      cur.submissions += st.submissions
      cur.ac += st.ac
      merged.set(d.date, cur)
    }
  }
  const heatRows = [...merged.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  const heatData = heatRows.map(([date, v]) => [date, v.submissions])
  const maxSub = Math.max(1, ...heatRows.map(([, v]) => v.submissions))
  mount('heatmap', heatRef.value, {
    tooltip: { formatter: (p: { value: [string, number] }) => `${p.value[0]}：${p.value[1]} 次提交` },
    visualMap: {
      min: 0, max: maxSub, show: false,
      inRange: { color: ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] },
    },
    calendar: {
      range: [heatRows[0]?.[0], heatRows.at(-1)?.[0]],
      cellSize: ['auto', 16],
      itemStyle: { color: cssVar('--card'), borderColor: cssVar('--border') },
      dayLabel: { color: muted, nameMap: 'ZH' },
      monthLabel: { color: muted },
      yearLabel: { show: false },
    },
    series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: heatData }],
  })

  const t = profile.value.trend
  mount('trend', trendRef.value, {
    tooltip: { trigger: 'axis' },
    legend: { data: ['提交', 'AC'], textStyle: { color: muted } },
    grid: { left: 40, right: 16, top: 32, bottom: 24 },
    xAxis: { type: 'category', data: t.map((d) => d.date.slice(5)), axisLabel: { color: muted } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted } },
    series: [
      { name: '提交', type: 'line', data: t.map((d) => d.submissions), smooth: true,
        areaStyle: { opacity: 0.15 }, itemStyle: { color: cssVar('--primary') } },
      { name: 'AC', type: 'line', data: t.map((d) => d.ac), smooth: true,
        itemStyle: { color: cssVar('--ac') } },
    ],
  })

  const pieData = Object.entries(profile.value.by_status).map(([name, value]) => ({ name, value }))
  mount('pie', pieRef.value, {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { color: muted } },
    series: [{
      type: 'pie', radius: ['45%', '70%'], center: ['50%', '45%'],
      data: pieData, label: { color: muted },
    }],
  })

  const tags = [...profile.value.tags].sort((a, b) => b.attempted - a.attempted).slice(0, 12)
  mount('tags', tagRef.value, {
    tooltip: { trigger: 'axis' },
    legend: { data: ['尝试', '解决'], textStyle: { color: muted } },
    grid: { left: 90, right: 16, top: 32, bottom: 24 },
    xAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted } },
    yAxis: { type: 'category', data: tags.map((x) => x.tag), axisLabel: { color: muted } },
    series: [
      { name: '尝试', type: 'bar', data: tags.map((x) => x.attempted), itemStyle: { color: muted } },
      { name: '解决', type: 'bar', data: tags.map((x) => x.solved), itemStyle: { color: cssVar('--ac') } },
    ],
  })
  resizeAll()
}

watchTheme(drawAll)

onMounted(async () => {
  await load()
  await loadExt()
  // restore the persisted platform selection after the profile arrives
  const saved = localStorage.getItem('oj_heat_platforms')
  if (saved) {
    try { heatPlatforms.value = JSON.parse(saved) as string[] } catch { /* ignore */ }
  }
})

const extPlatformNames = computed(() =>
  Object.keys(profile.value?.platforms?.[0]?.by_platform ?? {}))

function togglePlatform(p: string, checked: boolean) {
  if (checked) heatPlatforms.value = [...heatPlatforms.value, p]
  else heatPlatforms.value = heatPlatforms.value.filter((x) => x !== p)
  persistPlatforms()
}

function persistPlatforms() {
  localStorage.setItem('oj_heat_platforms', JSON.stringify(heatPlatforms.value))
  drawAll()
}

// why watch: navigating between /users/:id pages reuses this component.
watch(userId, load)

async function load() {
  if (!userId.value) return
  profile.value = await Users.profile(userId.value)
  // why nextTick: the v-if="profile" cards are not in the DOM until the next
  // render flush; without this the chart refs are undefined at draw time.
  await nextTick()
  drawAll()
}

const acRate = computed(() => {
  if (!profile.value) return '0'
  const total = Object.values(profile.value.by_status).reduce((a, b) => a + b, 0)
  return total === 0 ? '0' : Math.round(((profile.value.by_status['AC'] ?? 0) / total) * 100) + '%'
})

// ===== 外站 OJ 刷题数据（完整板块整合自 /external）：绑定管理 + 统计 +
// 导入外站题目。绑定/导入仅本人资料页；统计徽章+最近 AC 对任何资料页可见。
const isOwn = computed(() => route.path === '/profile')
const extBindings = ref<ExternalBinding[]>([])
const extPlatforms = ref<string[]>([])
const extBindPlatform = ref('')
const extBindHandle = ref('')
const extBusy = ref(false)
const extReport = ref<ExternalReport | null>(null)

// 导入外站题目
const impSource = ref('')
const impSources = ref<string[]>([])
const impID = ref('')
const impBusy = ref(false)
const impPreview = ref<{ title: string; url: string } | null>(null)

const extStats = computed(() => extReport.value?.by_platform ?? [])
const extRecentAc = computed(() => extReport.value?.recent_ac ?? [])

async function loadExt() {
  if (userId.value == null) return
  if (!isOwn.value) {
    // 他人资料页：只拉统计（不含绑定/管理）
    try {
      extReport.value = await External.report(userId.value)
    } catch {
      extReport.value = null
    }
    return
  }
  try {
    const p = await External.platforms()
    extBindings.value = p.bindings
    extPlatforms.value = p.platforms
    if (!extBindPlatform.value && extPlatforms.value.length) extBindPlatform.value = extPlatforms.value[0]
  } catch {
    /* 不阻塞资料页 */
  }
  try {
    extReport.value = await External.report(userId.value)
  } catch {
    extReport.value = null
  }
  try {
    impSources.value = (await External.importSources()).problem_sources ?? []
    if (!impSource.value && impSources.value.length) impSource.value = impSources.value[0]
  } catch {
    /* 插件列表失败不阻塞 */
  }
}

async function extBind() {
  if (!extBindPlatform.value || !extBindHandle.value.trim()) {
    toast.warning('请选择平台并填写用户名/ID')
    return
  }
  extBusy.value = true
  try {
    const r = await External.bind(extBindPlatform.value, extBindHandle.value.trim())
    if (r.error) toast.warning(`已绑定，但同步失败：${r.error}`)
    else toast.success(`已绑定并同步 ${r.stored} 条新记录`)
    extBindHandle.value = ''
    await loadExt()
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    extBusy.value = false
  }
}

async function extSync(row: ExternalBinding) {
  extBusy.value = true
  try {
    const r = await External.sync(row.platform)
    if (r.error) toast.warning(`同步失败：${r.error}`)
    else toast.success(`新增 ${r.stored} 条记录`)
    await loadExt()
  } finally {
    extBusy.value = false
  }
}

async function extUnbind(row: ExternalBinding) {
  if (!(await confirmDialog({
    title: '解绑',
    description: `解除 ${row.platform}（${row.handle}）的绑定？历史抓取记录保留。`,
    danger: true,
  }))) return
  await External.unbind(row.platform)
  toast.success('已解绑')
  await loadExt()
}

function impPreviewRun() {
  if (!impID.value.trim()) {
    toast.warning('请填写外部题号（如 1900A / P1001）')
    return
  }
  impBusy.value = true
  impPreview.value = null
  External.previewProblem(impSource.value, impID.value.trim())
    .then((r) => {
      impPreview.value = { title: r.meta.title, url: r.meta.url }
    })
    .catch((e) => toast.error(errMsg(e)))
    .finally(() => (impBusy.value = false))
}

async function impImport() {
  impBusy.value = true
  try {
    const r = await External.importProblem(impSource.value, impID.value.trim())
    if (r.needs_review) {
      toast.success('已导入并提交审核（审核通过后公开）。题面与样例已带入，测试数据需管理员/出题人补充。')
      impID.value = ''
      impPreview.value = null
    } else {
      toast.success(`已导入 #${r.problem.id}（外部题面，无测试数据，请补充后使用）`)
      router.push(`/problems/${r.problem.id}/edit`)
    }
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    impBusy.value = false
  }
}
</script>

<template>
  <div v-if="profile">
    <Card>
      <CardContent class="flex flex-wrap items-center gap-4 p-6">
        <div>
          <h2 class="m-0 text-xl font-semibold">{{ profile.user.nickname || profile.user.username }}</h2>
          <span class="text-muted-foreground">@{{ profile.user.username }}</span>
        </div>
        <Badge v-if="profile.user.role === 'admin'" variant="destructive">admin</Badge>
        <Badge v-else-if="profile.user.role === 'setter'">setter</Badge>
        <span class="flex-1" />
        <div class="flex gap-6 text-center">
          <Stat label="已解决题目" :value="profile.ac_problems" />
          <Stat label="尝试题目" :value="profile.tried_problems" />
          <Stat label="AC 率" :value="acRate" />
        </div>
      </CardContent>
    </Card>

    <Card class="mt-3">
      <CardContent class="p-6">
        <Alert
          v-for="err in chartErrors"
          :key="err"
          variant="error"
          :title="err"
          class="mb-2"
        />
        <div class="mb-1 flex flex-wrap items-center gap-3">
          <h4 class="m-0 text-base font-semibold">近一年活跃度</h4>
          <span class="text-xs text-muted-foreground">合并平台：</span>
          <Badge :variant="heatPlatforms.length === 0 ? 'ac' : 'secondary'">本站（始终包含）</Badge>
          <label
            v-for="p in extPlatformNames"
            :key="p"
            class="inline-flex cursor-pointer items-center gap-2 text-sm"
          >
            <Checkbox
              :model-value="heatPlatforms.includes(p)"
              @update:model-value="togglePlatform(p, $event)"
            />
            <span>{{ p }}</span>
          </label>
        </div>
        <div ref="heatRef" class="h-[180px] w-full" />
        <div v-if="extPlatformNames.length" class="text-xs text-muted-foreground">
          外部平台数据来自下方「外站 OJ 刷题数据」的账号绑定，每小时自动同步。
        </div>
      </CardContent>
    </Card>

    <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-[7fr_5fr]">
      <Card>
        <CardContent class="p-6">
          <h4 class="mb-2 text-base font-semibold">近 30 天趋势</h4>
          <div ref="trendRef" class="h-[280px] w-full" />
        </CardContent>
      </Card>
      <Card>
        <CardContent class="p-6">
          <h4 class="mb-2 text-base font-semibold">提交状态分布</h4>
          <div ref="pieRef" class="h-[280px] w-full" />
        </CardContent>
      </Card>
    </div>

    <Card class="mt-3">
      <CardContent class="p-6">
        <h4 class="mb-2 text-base font-semibold">标签强弱分布（按尝试排序）</h4>
        <div ref="tagRef" class="h-[300px] w-full" />
        <Alert
          v-if="profile.tags.length === 0"
          variant="info"
          title="暂无标签数据——提交的题目带有标签后，这里会展示强弱项分布"
        />
      </CardContent>
    </Card>

    <!-- 外站 OJ 刷题数据（完整板块，整合自 /external） -->
    <Card v-if="isOwn" class="mt-3">
      <CardContent class="space-y-4 p-6">
        <div class="flex flex-wrap items-center gap-3">
          <h4 class="m-0 text-base font-semibold">外站 OJ 刷题数据</h4>
          <span class="text-xs text-muted-foreground">
            绑定 Codeforces / 洛谷 / AtCoder / 牛客，每小时自动同步并合并进上方热力图
          </span>
        </div>

        <!-- 绑定表单 -->
        <div class="flex flex-wrap gap-2">
          <Select v-model="extBindPlatform" class="w-40">
            <SelectTrigger><SelectValue placeholder="平台" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="p in extPlatforms" :key="p" :value="p">{{ p }}</SelectItem>
            </SelectContent>
          </Select>
          <Input
            v-model="extBindHandle"
            class="w-[260px]"
            placeholder="平台用户名 / ID"
            @keyup.enter="extBind"
          />
          <Button :disabled="extBusy" @click="extBind">
            {{ extBusy ? '同步中…' : '绑定并立即同步' }}
          </Button>
        </div>

        <!-- 绑定表 -->
        <Table v-if="extBindings.length">
          <TableHeader>
            <TableRow>
              <TableHead class="w-[120px]">平台</TableHead>
              <TableHead>账号</TableHead>
              <TableHead class="w-[180px]">最近同步</TableHead>
              <TableHead class="w-[110px]">状态</TableHead>
              <TableHead class="w-[160px]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in extBindings" :key="row.platform">
              <TableCell>{{ row.platform }}</TableCell>
              <TableCell>{{ row.handle }}</TableCell>
              <TableCell>{{ row.synced_at ? new Date(row.synced_at).toLocaleString() : '从未' }}</TableCell>
              <TableCell>
                <span v-if="row.last_error" class="text-xs text-tle">{{ row.last_error }}</span>
                <Badge v-else variant="ac">正常</Badge>
              </TableCell>
              <TableCell>
                <div class="flex gap-1">
                  <Button size="sm" :disabled="extBusy" @click="extSync(row)">立即同步</Button>
                  <Button size="sm" variant="ghost" class="text-destructive" @click="extUnbind(row)">解绑</Button>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p v-else class="text-sm text-muted-foreground">
          还没有绑定外部平台账号——绑定后做题记录会合并进个人热力图。
        </p>

        <!-- 统计徽章 + 跨平台最近 AC -->
        <template v-if="extReport">
          <div class="flex flex-wrap gap-2">
            <Badge
              v-for="p in extStats"
              :key="p.platform"
              variant="secondary"
              class="px-3.5 py-2 text-sm"
            >
              {{ p.platform }}：提交 {{ p.submissions }} · AC {{ p.ac }} · 已解决 {{ p.solved }}
            </Badge>
          </div>
          <div v-if="extRecentAc.length">
            <h5 class="mb-2 mt-1 text-sm font-semibold">跨平台最近 AC</h5>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead class="w-[120px]">平台</TableHead>
                  <TableHead class="min-w-[220px]">题目</TableHead>
                  <TableHead class="w-[180px]">时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="row in extRecentAc" :key="`${row.platform}-${row.problem_id}-${row.at}`">
                  <TableCell>{{ row.platform }}</TableCell>
                  <TableCell>{{ row.problem_id }} {{ row.problem_name }}</TableCell>
                  <TableCell>{{ new Date(row.at * 1000).toLocaleString() }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </template>
      </CardContent>
    </Card>

    <!-- 他人资料页：外站统计（有数据才显示；管理仅本人） -->
    <Card v-if="!isOwn && extStats.length" class="mt-3">
      <CardContent class="p-6">
        <h4 class="mb-3 text-base font-semibold">外站 OJ 刷题统计</h4>
        <div class="flex flex-wrap gap-2">
          <Badge v-for="p in extStats" :key="p.platform" variant="secondary" class="px-3.5 py-2 text-sm">
            {{ p.platform }}：提交 {{ p.submissions }} · AC {{ p.ac }} · 已解决 {{ p.solved }}
          </Badge>
        </div>
        <div v-if="extRecentAc.length">
          <h5 class="mb-2 mt-4 text-sm font-semibold">跨平台最近 AC</h5>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-[120px]">平台</TableHead>
                <TableHead class="min-w-[220px]">题目</TableHead>
                <TableHead class="w-[180px]">时间</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="row in extRecentAc" :key="`${row.platform}-${row.problem_id}-${row.at}`">
                <TableCell>{{ row.platform }}</TableCell>
                <TableCell>{{ row.problem_id }} {{ row.problem_name }}</TableCell>
                <TableCell>{{ new Date(row.at * 1000).toLocaleString() }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>

    <!-- 导入外站题目（仅本人） -->
    <Card v-if="isOwn" class="mt-3">
      <CardContent class="space-y-3 p-6">
        <h4 class="m-0 text-base font-semibold">导入外站题目</h4>
        <div class="flex flex-wrap gap-2">
          <Select v-model="impSource" class="w-40">
            <SelectTrigger><SelectValue placeholder="平台" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="p in impSources" :key="p" :value="p">{{ p }}</SelectItem>
            </SelectContent>
          </Select>
          <Input
            v-model="impID"
            class="w-[240px]"
            placeholder="外部题号（如 1900A / P1001）"
            @keyup.enter="impPreviewRun"
          />
          <Button variant="outline" :disabled="impBusy" @click="impPreviewRun">预览题面</Button>
          <Button :disabled="impBusy || !impPreview" @click="impImport">
            {{ impBusy ? '导入中…' : '导入' }}
          </Button>
        </div>
        <div v-if="impPreview" class="rounded-md border border-border p-3 text-sm">
          <a :href="impPreview.url" target="_blank" rel="noreferrer" class="font-semibold text-primary hover:underline">
            {{ impPreview.title }}
          </a>
          <span class="ml-2 text-xs text-muted-foreground">原题链接（导入不抓取测试数据）</span>
        </div>
        <Alert variant="info"
          title="导入内容 = 题面 + 公开样例；测试数据不抓取，导入后标记「待补测试数据」，由出题人/管理员补充。一般用户导入的题需管理员审核通过后公开。" />
      </CardContent>
    </Card>

    <Card class="mt-3">
      <CardContent class="p-6">
        <h4 class="mb-2 text-base font-semibold">提交状态明细</h4>
        <p v-for="(n, s) in profile.by_status" :key="s" class="my-1 mr-4 inline-block">
          <StatusTag :status="s" /> × {{ n }}
        </p>
      </CardContent>
    </Card>
  </div>
</template>
