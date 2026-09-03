<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errMsg } from '../api/client'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Stat } from '@/components/ui/stat'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cssVar, useChart } from '@/composables/useChart'

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
const { chartErrors, mount, disposeAll, resizeAll, watchTheme } = useChart()
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
  disposeAll()
  const muted = cssVar('--muted-foreground')
  mount('trend', trendEl.value, {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 24, bottom: 24 },
    xAxis: { type: 'category', data: trend.value.map((d) => d.date.slice(5)), axisLabel: { color: muted } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted } },
    series: [{ type: 'line', data: trend.value.map((d) => d.count), smooth: true,
      areaStyle: { opacity: 0.15 }, itemStyle: { color: cssVar('--primary') } }],
  })
  mount('concurrency', concEl.value, {
    tooltip: { trigger: 'axis' },
    legend: { data: ['峰值并发', '提交数'], textStyle: { color: muted } },
    grid: { left: 40, right: 16, top: 32, bottom: 40 },
    xAxis: { type: 'category', data: concurrency.value.map((d) => d.time), axisLabel: { color: muted, rotate: 45 } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted } },
    series: [
      { name: '峰值并发', type: 'bar', data: concurrency.value.map((d) => d.peak), itemStyle: { color: cssVar('--tle') } },
      { name: '提交数', type: 'line', data: concurrency.value.map((d) => d.count), smooth: true, itemStyle: { color: cssVar('--primary') } },
    ],
  })
  resizeAll()
}

watchTheme(draw)

let timer: ReturnType<typeof setInterval> | null = null
onMounted(async () => {
  await load()
  timer = setInterval(load, 30000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <Alert v-if="errorText" variant="error" :title="errorText" class="mb-3" />
    <Alert
      v-for="err in chartErrors"
      :key="err"
      variant="error"
      :title="err"
      class="mb-3"
    />
    <div v-if="overview" class="grid grid-cols-2 gap-3 md:grid-cols-4">
      <Card><CardContent class="p-6"><Stat label="注册用户" :value="overview.users" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="今日提交" :value="overview.submissions_today" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="提交总量" :value="overview.submissions_total" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="判题队列" :value="overview.queue_length" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="在线判题机" :value="`${overview.daemons_online}/${overview.daemons}`" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="题目总数" :value="overview.problems" /></CardContent></Card>
      <Card><CardContent class="p-6"><Stat label="比赛总数" :value="overview.contests" /></CardContent></Card>
      <Card>
        <CardContent class="p-6">
          <Stat
            label="API 负载 (load1)"
            :value="overview.api_load.load1.toFixed(2)"
            :sub="`内存 ${Math.round(overview.api_load.mem_used_mb)}/${Math.round(overview.api_load.mem_total_mb)} MB`"
          />
        </CardContent>
      </Card>
    </div>

    <div class="mt-3 grid gap-3 md:grid-cols-2">
      <Card>
        <CardContent class="p-6">
          <h4 class="mb-2 text-base font-semibold">近 14 天提交趋势</h4>
          <div ref="trendEl" class="h-[260px] w-full" />
        </CardContent>
      </Card>
      <Card>
        <CardContent class="p-6">
          <h4 class="mb-2 text-base font-semibold">近 24 小时判题并发</h4>
          <div ref="concEl" class="h-[260px] w-full" />
        </CardContent>
      </Card>
    </div>

    <Card class="mt-3">
      <CardContent class="p-6">
        <h4 class="mb-2 text-base font-semibold">判题机负载</h4>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="min-w-[120px]">名称</TableHead>
              <TableHead class="w-[90px]">状态</TableHead>
              <TableHead class="w-[90px]">load1</TableHead>
              <TableHead class="min-w-[140px]">内存</TableHead>
              <TableHead class="w-[110px]">在判/容量</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in judgeLoad" :key="row.name">
              <TableCell>{{ row.name }}</TableCell>
              <TableCell>
                <Badge :variant="row.status === 'online' ? 'ac' : 'destructive'">{{ row.status }}</Badge>
              </TableCell>
              <TableCell>{{ row.load1 ? row.load1.toFixed(2) : '—' }}</TableCell>
              <TableCell>
                <Progress :value="row.mem_total_mb ? Math.round((row.mem_used_mb / row.mem_total_mb) * 100) : 0" />
              </TableCell>
              <TableCell>{{ row.active_tasks }}/{{ row.capacity }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p class="mt-2 text-xs text-muted-foreground">
          负载数据来自判题机心跳（每 15 秒），零值表示心跳版本较旧或非 Linux 环境
        </p>
      </CardContent>
    </Card>
  </div>
</template>
