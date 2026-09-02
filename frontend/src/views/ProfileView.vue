<script setup lang="ts">
import * as echarts from 'echarts/core'
import { BarChart, HeatmapChart, LineChart, PieChart } from 'echarts/charts'
import {
  CalendarComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Users, type UserProfile } from '../api/client'
import { useAuthStore } from '../stores/auth'
import StatusTag from '../components/StatusTag.vue'

echarts.use([
  BarChart,
  HeatmapChart,
  LineChart,
  PieChart,
  CalendarComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
  CanvasRenderer,
])

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
const charts: echarts.ECharts[] = []
// why surfaced in-page: chart init errors would otherwise be a silent blank
// section; showing them makes remote debugging possible without console.
const chartErrors = ref<string[]>([])
let resizeHandler: (() => void) | null = null

function mount(name: string, el: HTMLDivElement | undefined, option: echarts.EChartsCoreOption) {
  if (!el) {
    chartErrors.value.push(`${name}: 容器不存在`)
    return
  }
  try {
    const chart = echarts.init(el)
    chart.setOption(option)
    charts.push(chart)
  } catch (e) {
    chartErrors.value.push(`${name}: ${String(e)}`)
  }
}

function drawAll() {
  if (!profile.value) return
  charts.length = 0

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
      itemStyle: { color: '#161b22', borderColor: '#0d1117' },
      dayLabel: { color: '#8b949e', nameMap: 'ZH' },
      monthLabel: { color: '#8b949e' },
      yearLabel: { show: false },
    },
    series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: heatData }],
  })

  const t = profile.value.trend
  mount('trend', trendRef.value, {
    tooltip: { trigger: 'axis' },
    legend: { data: ['提交', 'AC'], textStyle: { color: '#909399' } },
    grid: { left: 40, right: 16, top: 32, bottom: 24 },
    xAxis: { type: 'category', data: t.map((d) => d.date.slice(5)), axisLabel: { color: '#909399' } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: '#909399' } },
    series: [
      { name: '提交', type: 'line', data: t.map((d) => d.submissions), smooth: true,
        areaStyle: { opacity: 0.15 }, itemStyle: { color: '#409eff' } },
      { name: 'AC', type: 'line', data: t.map((d) => d.ac), smooth: true,
        itemStyle: { color: '#67c23a' } },
    ],
  })

  const pieData = Object.entries(profile.value.by_status).map(([name, value]) => ({ name, value }))
  mount('pie', pieRef.value, {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { color: '#909399' } },
    series: [{
      type: 'pie', radius: ['45%', '70%'], center: ['50%', '45%'],
      data: pieData, label: { color: '#909399' },
    }],
  })

  const tags = [...profile.value.tags].sort((a, b) => b.attempted - a.attempted).slice(0, 12)
  mount('tags', tagRef.value, {
    tooltip: { trigger: 'axis' },
    legend: { data: ['尝试', '解决'], textStyle: { color: '#909399' } },
    grid: { left: 90, right: 16, top: 32, bottom: 24 },
    xAxis: { type: 'value', minInterval: 1, axisLabel: { color: '#909399' } },
    yAxis: { type: 'category', data: tags.map((x) => x.tag), axisLabel: { color: '#909399' } },
    series: [
      { name: '尝试', type: 'bar', data: tags.map((x) => x.attempted), itemStyle: { color: '#8a94a6' } },
      { name: '解决', type: 'bar', data: tags.map((x) => x.solved), itemStyle: { color: '#67c23a' } },
    ],
  })
  for (const c of charts) c.resize()
}

onMounted(async () => {
  await load()
  // restore the persisted platform selection after the profile arrives
  const saved = localStorage.getItem('oj_heat_platforms')
  if (saved) {
    try { heatPlatforms.value = JSON.parse(saved) as string[] } catch { /* ignore */ }
  }
  resizeHandler = () => { for (const c of charts) c.resize() }
  window.addEventListener('resize', resizeHandler)
})

const extPlatformNames = computed(() =>
  Object.keys(profile.value?.platforms?.[0]?.by_platform ?? {}))

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

onBeforeUnmount(() => {
  if (resizeHandler) window.removeEventListener('resize', resizeHandler)
  for (const c of charts) c.dispose()
  charts.length = 0
})

const acRate = computed(() => {
  if (!profile.value) return '0'
  const total = Object.values(profile.value.by_status).reduce((a, b) => a + b, 0)
  return total === 0 ? '0' : Math.round(((profile.value.by_status['AC'] ?? 0) / total) * 100) + '%'
})
</script>

<template>
  <div v-if="profile">
    <el-card>
      <div style="display: flex; align-items: center; gap: 16px; flex-wrap: wrap">
        <div>
          <h2 style="margin: 0">{{ profile.user.nickname || profile.user.username }}</h2>
          <span style="color: #909399">@{{ profile.user.username }}</span>
        </div>
        <el-tag v-if="profile.user.role === 'admin'" type="danger">admin</el-tag>
        <el-tag v-else-if="profile.user.role === 'setter'">setter</el-tag>
        <span style="flex: 1" />
        <div style="display: flex; gap: 24px; text-align: center">
          <div><el-statistic title="已解决题目" :value="profile.ac_problems" /></div>
          <div><el-statistic title="尝试题目" :value="profile.tried_problems" /></div>
          <div><el-statistic title="AC 率" :value="acRate" /></div>
        </div>
      </div>
    </el-card>

    <el-card style="margin-top: 12px">
      <el-alert
        v-for="err in chartErrors"
        :key="err"
        :title="err"
        type="error"
        :closable="false"
        style="margin-bottom: 8px"
      />
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 4px">
        <h4 style="margin: 0">近一年活跃度</h4>
        <span style="color: #909399; font-size: 12px">合并平台：</span>
        <el-tag size="small" :type="heatPlatforms.length === 0 ? 'success' : 'info'">本站（始终包含）</el-tag>
        <el-checkbox-group v-model="heatPlatforms" style="display: inline-flex; gap: 8px" @change="persistPlatforms">
          <el-checkbox v-for="p in extPlatformNames" :key="p" :value="p" :label="p" />
        </el-checkbox-group>
      </div>
      <div ref="heatRef" style="width: 100%; height: 180px" />
      <div v-if="extPlatformNames.length" style="color: #909399; font-size: 12px">
        外部平台数据来自「<router-link to="/external">刷题统计</router-link>」的账号绑定，每小时自动同步。
      </div>
    </el-card>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24" :md="14">
        <el-card><h4 style="margin-top: 0">近 30 天趋势</h4><div ref="trendRef" style="width: 100%; height: 280px" /></el-card>
      </el-col>
      <el-col :xs="24" :md="10">
        <el-card><h4 style="margin-top: 0">提交状态分布</h4><div ref="pieRef" style="width: 100%; height: 280px" /></el-card>
      </el-col>
    </el-row>

    <el-card style="margin-top: 12px">
      <h4 style="margin-top: 0">标签强弱分布（按尝试排序）</h4>
      <div ref="tagRef" style="width: 100%; height: 300px" />
      <el-alert
        v-if="profile.tags.length === 0"
        title="暂无标签数据——提交的题目带有标签后，这里会展示强弱项分布"
        type="info"
        :closable="false"
      />
    </el-card>

    <el-card style="margin-top: 12px">
      <h4 style="margin-top: 0">提交状态明细</h4>
      <p v-for="(n, s) in profile.by_status" :key="s" style="margin: 4px 0; display: inline-block; margin-right: 16px">
        <StatusTag :status="s" /> × {{ n }}
      </p>
    </el-card>
  </div>
</template>
