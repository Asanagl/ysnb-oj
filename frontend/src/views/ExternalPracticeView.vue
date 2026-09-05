<script setup lang="ts">
// 刷题统计报表：绑定外部平台账号（CF/洛谷/AtCoder/牛客）→ 服务端定时同步
// → 合并热力图 + 各平台统计 + 跨平台最近 AC。默认查看自己；他人主页路由
// /users/:id 仍走 ProfileView，这里加 ?user= 供未来扩展。
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { errMsg, External, Users, type ExternalBinding, type ExternalReport } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cssVar, useChart } from '@/composables/useChart'
import { confirmDialog } from '@/lib/confirm'
import { toast } from '@/lib/toast'

const auth = useAuthStore()
const router = useRouter()
const userId = computed(() => auth.user?.id ?? 0)
const report = ref<ExternalReport | null>(null)
const bindings = ref<ExternalBinding[]>([])
const platforms = ref<string[]>([])
const bindPlatform = ref('')
const bindHandle = ref('')
const busy = ref(false)
const heatRef = ref<HTMLDivElement>()
const { chartErrors, mount, disposeAll, resizeAll, watchTheme } = useChart()

async function load() {
  const p = await External.platforms()
  bindings.value = p.bindings
  platforms.value = p.platforms
  if (!bindPlatform.value && platforms.value.length) bindPlatform.value = platforms.value[0]
  report.value = await External.report(userId.value)
  await nextTick(drawHeat)
}

onMounted(() => {
  load()
  loadSources()
})

function drawHeat() {
  if (!report.value || !heatRef.value) return
  disposeAll()
  const muted = cssVar('--muted-foreground')
  const act = report.value.activity
  const maxSub = Math.max(1, ...act.map((d) => d.submissions))
  mount('heatmap', heatRef.value, {
    tooltip: { formatter: (p: { value: [string, number] }) => `${p.value[0]}：${p.value[1]} 次提交` },
    visualMap: {
      min: 0, max: maxSub, show: false,
      inRange: { color: ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] },
    },
    calendar: {
      range: [act[0]?.date, act.at(-1)?.date],
      cellSize: ['auto', 16],
      itemStyle: { color: cssVar('--card'), borderColor: cssVar('--border') },
      dayLabel: { color: muted, nameMap: 'ZH' },
      monthLabel: { color: muted },
      yearLabel: { show: false },
    },
    series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: act.map((d) => [d.date, d.submissions]) }],
  })
  resizeAll()
}

watchTheme(drawHeat)

async function bind() {
  if (!bindPlatform.value || !bindHandle.value.trim()) {
    toast.warning('请选择平台并填写用户名/ID')
    return
  }
  busy.value = true
  try {
    const r = await External.bind(bindPlatform.value, bindHandle.value.trim())
    if (r.error) toast.warning(`已绑定，但同步失败：${r.error}`)
    else toast.success(`已绑定并同步 ${r.stored} 条新记录`)
    bindHandle.value = ''
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

async function sync(row: ExternalBinding) {
  busy.value = true
  try {
    const r = await External.sync(row.platform)
    if (r.error) toast.warning(`同步失败：${r.error}`)
    else toast.success(`新增 ${r.stored} 条记录`)
    await load()
    await nextTick(drawHeat)
  } finally {
    busy.value = false
  }
}

async function unbind(row: ExternalBinding) {
  if (!(await confirmDialog({
    title: '解绑',
    description: `解除 ${row.platform}（${row.handle}）的绑定？历史抓取记录保留。`,
    danger: true,
  }))) return
  await External.unbind(row.platform)
  await load()
  await nextTick(drawHeat)
}

// myStats only exists for own profile; external report drives this page.
const platformTag = (p: string) => p

// keep Users import meaningful (used by callers navigating here from profile)
void Users

// ===== 外站题目导入（全员）=====
const impSource = ref('')
const impSources = ref<string[]>([])
const impID = ref('')
const impBusy = ref(false)
const impPreview = ref<{ title: string; url: string } | null>(null)

async function loadSources() {
  if (impSources.value.length) return
  try {
    impSources.value = (await External.importSources()).problem_sources ?? []
    if (!impSource.value && impSources.value.length) impSource.value = impSources.value[0]
  } catch {
    /* 插件列表拉取失败不阻塞页面 */
  }
}

async function impPreviewRun() {
  if (!impID.value.trim()) {
    toast.warning('请填写外部题号（如 1900A / P1001）')
    return
  }
  impBusy.value = true
  impPreview.value = null
  try {
    const r = await External.previewProblem(impSource.value, impID.value.trim())
    impPreview.value = { title: r.meta.title, url: r.meta.url }
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    impBusy.value = false
  }
}

async function impImport() {
  impBusy.value = true
  try {
    const r = await External.importProblem(impSource.value, impID.value.trim())
    if (r.needs_review) {
      toast.success(`已导入并提交审核（审核通过后公开）。题面与样例已带入，测试数据需管理员/出题人补充。`)
    } else {
      toast.success(`已导入 #${r.problem.id}（外部题面，无测试数据，请补充后使用）`)
      router.push(`/problems/${r.problem.id}/edit`)
      return
    }
    impID.value = ''
    impPreview.value = null
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    impBusy.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-[1000px]">
    <Card class="mb-4">
      <CardHeader>
        <CardTitle>导入外站题目</CardTitle>
      </CardHeader>
      <CardContent>
        <div class="mb-3 flex flex-wrap gap-2">
          <Select v-model="impSource">
            <SelectTrigger class="w-40"><SelectValue placeholder="平台" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="p in impSources" :key="p" :value="p">{{ p }}</SelectItem>
            </SelectContent>
          </Select>
          <Input v-model="impID" class="w-[240px]"
            placeholder="外部题号（如 1900A / P1001）" @keyup.enter="impPreviewRun" />
          <Button variant="outline" :disabled="impBusy" @click="impPreviewRun">预览题面</Button>
          <Button :disabled="impBusy || !impPreview" @click="impImport">
            {{ impBusy ? '导入中…' : '导入' }}
          </Button>
        </div>
        <div v-if="impPreview" class="mb-3 rounded-md border border-border p-3 text-sm">
          <a :href="impPreview.url" target="_blank" rel="noreferrer" class="font-semibold text-primary hover:underline">
            {{ impPreview.title }}
          </a>
          <span class="ml-2 text-xs text-muted-foreground">原题链接（导入不抓取测试数据）</span>
        </div>
        <Alert variant="info"
          title="导入内容 = 题面 + 公开样例；测试数据不抓取，导入后标记「待补测试数据」，由出题人/管理员补充。一般用户导入的题需管理员审核通过后公开。" />
      </CardContent>
    </Card>

    <Card class="mb-4">
      <CardHeader>
        <CardTitle>外部平台绑定（刷题统计报表）</CardTitle>
      </CardHeader>
      <CardContent>
        <div class="mb-3 flex flex-wrap gap-2">
          <Select v-model="bindPlatform">
            <SelectTrigger class="w-40">
              <SelectValue placeholder="平台" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="p in platforms" :key="p" :value="p">{{ platformTag(p) }}</SelectItem>
            </SelectContent>
          </Select>
          <Input v-model="bindHandle" class="w-[260px]"
            :placeholder="bindPlatform === 'nowcoder' ? '牛客数字 ID 或主页链接' : bindPlatform === 'luogu' ? '洛谷用户名' : '平台用户名'"
            @keyup.enter="bind" />
          <Button :disabled="busy" @click="bind">{{ busy ? '同步中…' : '绑定并立即同步' }}</Button>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-[120px]">平台</TableHead>
              <TableHead>账号</TableHead>
              <TableHead class="w-[180px]">最近同步</TableHead>
              <TableHead class="min-w-[200px]">状态</TableHead>
              <TableHead class="w-[160px]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in bindings" :key="row.platform">
              <TableCell>{{ row.platform }}</TableCell>
              <TableCell>{{ row.handle }}</TableCell>
              <TableCell>{{ row.synced_at ? new Date(row.synced_at).toLocaleString() : '从未' }}</TableCell>
              <TableCell>
                <span v-if="row.last_error" class="text-xs text-tle">{{ row.last_error }}</span>
                <Badge v-else variant="ac">正常</Badge>
              </TableCell>
              <TableCell>
                <div class="flex gap-1">
                  <Button size="sm" :disabled="busy" @click="sync(row)">立即同步</Button>
                  <Button size="sm" variant="ghost" class="text-destructive" @click="unbind(row)">解绑</Button>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <Alert variant="info" class="mt-3"
          title="同步频率：每小时自动增量拉取一次（只读公开数据）。洛谷/牛客反爬较强，失败会在上方显示原因，稍后重试即可。" />
      </CardContent>
    </Card>

    <Card v-if="report" class="mb-4">
      <CardHeader>
        <CardTitle>刷题统计（合并全部已绑定平台）</CardTitle>
      </CardHeader>
      <CardContent>
        <div class="mb-4 flex flex-wrap gap-4">
          <Badge v-for="p in report.by_platform" :key="p.platform" variant="secondary" class="px-3.5 py-2 text-sm">
            {{ p.platform }}：提交 {{ p.submissions }} · AC {{ p.ac }} · 已解决 {{ p.solved }}
          </Badge>
          <Badge v-if="!report.by_platform.length" variant="secondary" class="px-3.5 py-2 text-sm">还没有外部平台数据，先绑定一个账号</Badge>
        </div>
        <div ref="heatRef" class="h-[180px] w-full" />
        <Alert v-for="err in chartErrors" :key="err" variant="error" class="mt-2" :title="err" />
        <h4 class="mb-2 mt-4 text-sm font-semibold">跨平台最近 AC</h4>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-[120px]">平台</TableHead>
              <TableHead class="min-w-[220px]">题目</TableHead>
              <TableHead class="w-[180px]">时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in report.recent_ac" :key="`${row.platform}-${row.problem_id}-${row.at}`">
              <TableCell>{{ row.platform }}</TableCell>
              <TableCell>{{ row.problem_id }} {{ row.problem_name }}</TableCell>
              <TableCell>{{ new Date(row.at * 1000).toLocaleString() }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
