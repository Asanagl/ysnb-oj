<script setup lang="ts">
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errMsg } from '../api/client'

echarts.use([BarChart, LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

interface Overview {
  users: number
  submissions_total: number
  submissions_today: number
  ac_total: number
  queue_length: number
  daemons: number
  daemons_online: number
  contests: number
  problems: number
  api_load: { load1: number; mem_used_mb: number; mem_total_mb: number }
}
interface LoadRow {
  name: string
  status: string
  active_tasks: number
  capacity: number
  load1: number
  mem_used_mb: number
  mem_total_mb: number
  last_heartbeat: string | null
}

const overview = ref<Overview | null>(null)
const trend = ref<{ date: string; count: number }[]>([])
const concurrency = ref<{ time: string; peak: number; count: number }[]>([])
const judgeLoad = ref<LoadRow[]>([])
const trendEl = ref<HTMLDivElement>()
const concEl = ref<HTMLDivElement>()
const charts: echarts.ECharts[] = []
const errorText = ref('')

async function load() {
  try {
    overview.value = (await api.get('/admin/stats/overview')).data
    trend.value = (await api.get('/admin/stats/trend?days=14')).data.days
    concurrency.value = (await api.get('/admin/stats/concurrency?hours=24')).data.hours
    const loadResp = (await api.get('/admin/stats/load')).data as { judge: LoadRow[] }
    judgeLoad.value = loadResp.judge
    draw()
  } catch (e) {
    errorText.value = errMsg(e)
  }
}

function draw() {
  charts.length = 0
  if (trendEl.value) {
    const c = echarts.init(trendEl.value)
    c.setOption({
      tooltip: { trigger: 'axis' },
      grid: { left: 40, right: 16, top: 24, bottom: 24 },
      xAxis: { type: 'category', data: trend.value.map((d) => d.date.slice(5)), axisLabel: { color: '#909399' } },
      yAxis: { type: 'value', minInterval: 1, axisLabel: { color: '#909399' } },
      series: [{ type: 'line', data: trend.value.map((d) => d.count), smooth: true,
        areaStyle: { opacity: 0.15 }, itemStyle: { color: '#409eff' } }],
    })
    charts.push(c)
  }
  if (concEl.value) {
    const c = echarts.init(concEl.value)
    c.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['峰值并发', '提交数'], textStyle: { color: '#909399' } },
      grid: { left: 40, right: 16, top: 32, bottom: 40 },
      xAxis: { type: 'category', data: concurrency.value.map((d) => d.time), axisLabel: { color: '#909399', rotate: 45 } },
      yAxis: { type: 'value', minInterval: 1, axisLabel: { color: '#909399' } },
      series: [
        { name: '峰值并发', type: 'bar', data: concurrency.value.map((d) => d.peak), itemStyle: { color: '#e6a23c' } },
        { name: '提交数', type: 'line', data: concurrency.value.map((d) => d.count), smooth: true, itemStyle: { color: '#409eff' } },
      ],
    })
    charts.push(c)
  }
}

function resize() {
  for (const c of charts) c.resize()
}

let timer: ReturnType<typeof setInterval> | null = null
onMounted(async () => {
  await load()
  window.addEventListener('resize', resize)
  timer = setInterval(load, 30000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  window.removeEventListener('resize', resize)
  for (const c of charts) c.dispose()
  charts.length = 0
})
</script>

<template>
  <div>
    <el-alert v-if="errorText" :title="errorText" type="error" :closable="false" style="margin-bottom: 12px" />
    <el-row v-if="overview" :gutter="12">
      <el-col :xs="12" :md="6"><el-card><el-statistic title="注册用户" :value="overview.users" /></el-card></el-col>
      <el-col :xs="12" :md="6"><el-card><el-statistic title="今日提交" :value="overview.submissions_today" /></el-card></el-col>
      <el-col :xs="12" :md="6"><el-card><el-statistic title="提交总量" :value="overview.submissions_total" /></el-card></el-col>
      <el-col :xs="12" :md="6"><el-card><el-statistic title="判题队列" :value="overview.queue_length" /></el-card></el-col>
      <el-col :xs="12" :md="6" style="margin-top: 12px"><el-card><el-statistic title="在线判题机" :value="`${overview.daemons_online}/${overview.daemons}`" /></el-card></el-col>
      <el-col :xs="12" :md="6" style="margin-top: 12px"><el-card><el-statistic title="题目总数" :value="overview.problems" /></el-card></el-col>
      <el-col :xs="12" :md="6" style="margin-top: 12px"><el-card><el-statistic title="比赛总数" :value="overview.contests" /></el-card></el-col>
      <el-col :xs="12" :md="6" style="margin-top: 12px">
        <el-card>
          <el-statistic title="API 负载 (load1)" :value="overview.api_load.load1" :precision="2" />
          <p style="color: #909399; font-size: 12px; margin: 4px 0 0">
            内存 {{ Math.round(overview.api_load.mem_used_mb) }}/{{ Math.round(overview.api_load.mem_total_mb) }} MB
          </p>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24" :md="12">
        <el-card><h4 style="margin-top: 0">近 14 天提交趋势</h4><div ref="trendEl" style="width: 100%; height: 260px" /></el-card>
      </el-col>
      <el-col :xs="24" :md="12">
        <el-card><h4 style="margin-top: 0">近 24 小时判题并发</h4><div ref="concEl" style="width: 100%; height: 260px" /></el-card>
      </el-col>
    </el-row>

    <el-card style="margin-top: 12px">
      <h4 style="margin-top: 0">判题机负载</h4>
      <el-table :data="judgeLoad" size="small">
        <el-table-column label="名称" prop="name" min-width="120" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'online' ? 'success' : 'danger'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="load1" width="90">
          <template #default="{ row }">{{ row.load1 ? row.load1.toFixed(2) : '—' }}</template>
        </el-table-column>
        <el-table-column label="内存" min-width="140">
          <template #default="{ row }">
            <el-progress
              :percentage="row.mem_total_mb ? Math.round((row.mem_used_mb / row.mem_total_mb) * 100) : 0"
              :stroke-width="12"
            />
          </template>
        </el-table-column>
        <el-table-column label="在判/容量" width="110">
          <template #default="{ row }">{{ row.active_tasks }}/{{ row.capacity }}</template>
        </el-table-column>
      </el-table>
      <p style="color: #909399; font-size: 12px; margin: 8px 0 0">
        负载数据来自判题机心跳（每 15 秒），零值表示心跳版本较旧或非 Linux 环境
      </p>
    </el-card>
  </div>
</template>
