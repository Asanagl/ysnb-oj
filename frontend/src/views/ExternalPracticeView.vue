<script setup lang="ts">
// 刷题统计报表：绑定外部平台账号（CF/洛谷/AtCoder/牛客）→ 服务端定时同步
// → 合并热力图 + 各平台统计 + 跨平台最近 AC。默认查看自己；他人主页路由
// /users/:id 仍走 ProfileView，这里加 ?user= 供未来扩展。
import * as echarts from 'echarts/core'
import { HeatmapChart } from 'echarts/charts'
import { CalendarComponent, TooltipComponent, VisualMapComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { errMsg, External, Users, type ExternalBinding, type ExternalReport } from '../api/client'
import { useAuthStore } from '../stores/auth'

echarts.use([HeatmapChart, CalendarComponent, TooltipComponent, VisualMapComponent, CanvasRenderer])

const auth = useAuthStore()
const userId = computed(() => auth.user?.id ?? 0)
const report = ref<ExternalReport | null>(null)
const bindings = ref<ExternalBinding[]>([])
const platforms = ref<string[]>([])
const bindPlatform = ref('')
const bindHandle = ref('')
const busy = ref(false)
const heatRef = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null
let resizeHandler: (() => void) | null = null

async function load() {
  const p = await External.platforms()
  bindings.value = p.bindings
  platforms.value = p.platforms
  if (!bindPlatform.value && platforms.value.length) bindPlatform.value = platforms.value[0]
  report.value = await External.report(userId.value)
  await nextTick(drawHeat)
}

onMounted(load)

function drawHeat() {
  if (!report.value || !heatRef.value) return
  chart?.dispose()
  chart = echarts.init(heatRef.value)
  const act = report.value.activity
  const maxSub = Math.max(1, ...act.map((d) => d.submissions))
  chart.setOption({
    tooltip: { formatter: (p: { value: [string, number] }) => `${p.value[0]}：${p.value[1]} 次提交` },
    visualMap: {
      min: 0, max: maxSub, show: false,
      inRange: { color: ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] },
    },
    calendar: {
      range: [act[0]?.date, act.at(-1)?.date],
      cellSize: ['auto', 16],
      itemStyle: { color: '#161b22', borderColor: '#0d1117' },
      dayLabel: { color: '#8b949e', nameMap: 'ZH' },
      monthLabel: { color: '#8b949e' },
      yearLabel: { show: false },
    },
    series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: act.map((d) => [d.date, d.submissions]) }],
  })
  resizeHandler = () => chart?.resize()
  window.addEventListener('resize', resizeHandler)
}

onBeforeUnmount(() => {
  if (resizeHandler) window.removeEventListener('resize', resizeHandler)
  chart?.dispose()
})

async function bind() {
  if (!bindPlatform.value || !bindHandle.value.trim()) {
    ElMessage.warning('请选择平台并填写用户名/ID')
    return
  }
  busy.value = true
  try {
    const r = await External.bind(bindPlatform.value, bindHandle.value.trim())
    if (r.error) ElMessage.warning(`已绑定，但同步失败：${r.error}`)
    else ElMessage.success(`已绑定并同步 ${r.stored} 条新记录`)
    bindHandle.value = ''
    await load()
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

async function sync(row: ExternalBinding) {
  busy.value = true
  try {
    const r = await External.sync(row.platform)
    if (r.error) ElMessage.warning(`同步失败：${r.error}`)
    else ElMessage.success(`新增 ${r.stored} 条记录`)
    await load()
    await nextTick(drawHeat)
  } finally {
    busy.value = false
  }
}

async function unbind(row: ExternalBinding) {
  await ElMessageBox.confirm(`解除 ${row.platform}（${row.handle}）的绑定？历史抓取记录保留。`, '解绑')
  await External.unbind(row.platform)
  await load()
  await nextTick(drawHeat)
}

// myStats only exists for own profile; external report drives this page.
const platformTag = (p: string) => p

// keep Users import meaningful (used by callers navigating here from profile)
void Users
</script>

<template>
  <div style="max-width: 1000px; margin: 0 auto">
    <el-card style="margin-bottom: 16px">
      <template #header>外部平台绑定（刷题统计报表）</template>
      <div style="display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap">
        <el-select v-model="bindPlatform" style="width: 160px" placeholder="平台">
          <el-option v-for="p in platforms" :key="p" :label="platformTag(p)" :value="p" />
        </el-select>
        <el-input v-model="bindHandle" style="width: 260px"
          :placeholder="bindPlatform === 'nowcoder' ? '牛客数字 ID 或主页链接' : bindPlatform === 'luogu' ? '洛谷用户名' : '平台用户名'"
          @keyup.enter="bind" />
        <el-button type="primary" :loading="busy" @click="bind">绑定并立即同步</el-button>
      </div>
      <el-table :data="bindings" size="small">
        <el-table-column label="平台" prop="platform" width="120" />
        <el-table-column label="账号" prop="handle" />
        <el-table-column label="最近同步" width="180">
          <template #default="{ row }">{{ row.synced_at ? new Date(row.synced_at).toLocaleString() : '从未' }}</template>
        </el-table-column>
        <el-table-column label="状态" min-width="200">
          <template #default="{ row }">
            <span v-if="row.last_error" style="color: #e6a23c; font-size: 12px">{{ row.last_error }}</span>
            <el-tag v-else type="success" size="small">正常</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ row }">
            <el-button size="small" :disabled="busy" @click="sync(row)">立即同步</el-button>
            <el-button size="small" type="danger" text @click="unbind(row)">解绑</el-button>
          </template>
        </el-table-column>
      </el-table>
 <el-alert type="info" :closable="false" style="margin-top: 12px"
        title="同步频率：每小时自动增量拉取一次（只读公开数据）。洛谷/牛客反爬较强，失败会在上方显示原因，稍后重试即可。" />
    </el-card>

    <el-card v-if="report" style="margin-bottom: 16px">
      <template #header>刷题统计（合并全部已绑定平台）</template>
      <div style="display: flex; gap: 16px; flex-wrap: wrap; margin-bottom: 16px">
        <el-tag v-for="p in report.by_platform" :key="p.platform" size="large" style="padding: 8px 14px">
          {{ p.platform }}：提交 {{ p.submissions }} · AC {{ p.ac }} · 已解决 {{ p.solved }}
        </el-tag>
        <el-tag v-if="!report.by_platform.length" type="info" size="large">还没有外部平台数据，先绑定一个账号</el-tag>
      </div>
      <div ref="heatRef" style="width: 100%; height: 180px" />
      <h4 style="margin: 16px 0 8px">跨平台最近 AC</h4>
      <el-table :data="report.recent_ac" size="small">
        <el-table-column label="平台" prop="platform" width="120" />
        <el-table-column label="题目" min-width="220">
          <template #default="{ row }">{{ row.problem_id }} {{ row.problem_name }}</template>
        </el-table-column>
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ new Date(row.at * 1000).toLocaleString() }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>