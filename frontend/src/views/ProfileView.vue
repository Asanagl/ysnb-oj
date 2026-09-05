<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Users, errMsg, External, type ExternalBinding, type UserProfile } from '../api/client'
import { useAuthStore } from '../stores/auth'
import StatusTag from '../components/StatusTag.vue'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Stat } from '@/components/ui/stat'
import { cssVar, useChart } from '@/composables/useChart'
import { toast } from '@/lib/toast'

const route = useRoute()
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

// ===== 外站 OJ 刷题数据（仅本人资料页显示的快捷入口）=====
const isOwn = computed(() => route.path === '/profile')
const extBindings = ref<ExternalBinding[]>([])
const extPlatforms = ref<string[]>([])
const extBindPlatform = ref('')
const extBindHandle = ref('')
const extBusy = ref(false)

async function loadExt() {
  if (!isOwn.value) return
  try {
    const p = await External.platforms()
    extBindings.value = p.bindings
    extPlatforms.value = p.platforms
    if (!extBindPlatform.value && extPlatforms.value.length) extBindPlatform.value = extPlatforms.value[0]
  } catch {
    /* 不阻塞资料页 */
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
          外部平台数据来自「<router-link to="/external" class="text-primary hover:underline">刷题统计</router-link>」的账号绑定，每小时自动同步。
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

    <Card v-if="isOwn" class="mt-3">
      <CardContent class="p-6">
        <div class="mb-3 flex flex-wrap items-center gap-3">
          <h4 class="m-0 text-base font-semibold">外站 OJ 刷题数据</h4>
          <span class="text-xs text-muted-foreground">
            绑定 Codeforces / 洛谷 / AtCoder / 牛客，每小时自动同步并合并进上方热力图
          </span>
          <span class="flex-1" />
          <router-link to="/external" class="text-sm text-primary hover:underline">
            完整报表与题目导入 →
          </router-link>
        </div>
        <div class="mb-3 flex flex-wrap gap-2">
          <select
            v-model="extBindPlatform"
            class="h-9 rounded-md border border-border bg-card px-3 text-sm"
          >
            <option v-for="p in extPlatforms" :key="p" :value="p">{{ p }}</option>
          </select>
          <Input
            v-model="extBindHandle"
            class="w-[240px]"
            placeholder="平台用户名 / ID"
            @keyup.enter="extBind"
          />
          <Button :disabled="extBusy" @click="extBind">
            {{ extBusy ? '同步中…' : '绑定并立即同步' }}
          </Button>
        </div>
        <div v-if="extBindings.length" class="flex flex-wrap gap-2">
          <Badge
            v-for="row in extBindings"
            :key="row.platform"
            :variant="row.last_error ? 'tle' : 'ac'"
            class="cursor-pointer px-3 py-1.5"
            :title="row.last_error ? `同步失败：${row.last_error}（点击重试）` : `最近同步 ${row.synced_at ? new Date(row.synced_at).toLocaleString() : '从未'}（点击重试）`"
            @click="extSync(row)"
          >
            {{ row.platform }} · {{ row.handle }}
          </Badge>
        </div>
        <p v-else class="text-sm text-muted-foreground">
          还没有绑定外部平台账号——绑定后做题记录会合并进个人热力图。
        </p>
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
