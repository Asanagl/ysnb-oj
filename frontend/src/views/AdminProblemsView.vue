<script setup lang="ts">
// 题目管理列表：新建/编辑跳转到独立全页编辑器 ProblemEditorView。
// 「导入题目包」就地解析 zip（自有格式 / DOMjudge / Hydro），导入后跳转编辑。
// 「外部题面导入」走插件爬虫（CF/洛谷），仅题面+公开样例，无测试数据。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { errMsg, External, Problems } from '../api/client'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Alert } from '@/components/ui/alert'
import { Pagination } from '@/components/ui/pagination'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const router = useRouter()

const items = ref<Awaited<ReturnType<typeof Problems.list>>['items']>([])
const total = ref(0)
const page = ref(1)
const importFile = ref<File | null>(null)
const importing = ref(false)

// external import dialog state
const extVisible = ref(false)
const extSources = ref<string[]>([])
const extSource = ref('codeforces')
const extID = ref('')
const extPreview = ref<{ title: string; url: string; statement_md: string; notes: string[] } | null>(null)
const extBusy = ref(false)

async function load() {
  const r = await Problems.list({ page: page.value, size: 20 })
  items.value = r.items
  total.value = r.total
}
onMounted(load)

function onPage(p: number) {
  page.value = p
  load()
}

function openCreate() {
  router.push('/problems/new')
}

function openEdit(row: { id: number }) {
  router.push(`/problems/${row.id}/edit`)
}

async function importPackage() {
  if (!importFile.value) {
    toast.warning('请先选择题目包 zip')
    return
  }
  importing.value = true
  try {
    const r = await Problems.importPackage(importFile.value)
    const notes = r.notes?.length ? `；${r.notes.join('；')}` : ''
    toast.success(`已导入 #${r.problem.id}（${r.stored} 个测试点，格式 ${r.format}）${notes}`)
    router.push(`/problems/${r.problem.id}/edit`)
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    importing.value = false
  }
}

async function openExternal() {
  extVisible.value = true
  if (!extSources.value.length) {
    extSources.value = (await Problems.externalSources()).problem_sources ?? []
  }
}

// preview then import in two steps; import is the only write.
// 注意：meta.statement_md 可能为空（CF 等平台反爬/题面抓取失败）——
// 必须兜底为空串，否则模板里 .slice() 抛错导致预览面板永远不出现。
async function extPreviewRun() {
  if (!extID.value.trim()) {
    toast.warning('请填写外部题号（如 1900A / P1001）')
    return
  }
  extBusy.value = true
  extPreview.value = null
  try {
    const r = await External.previewProblem(extSource.value, extID.value.trim())
    extPreview.value = {
      title: r.meta.title, url: r.meta.url,
      statement_md: r.meta.statement_md ?? '', notes: r.meta.notes ?? [],
    }
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    extBusy.value = false
  }
}

async function extImport() {
  extBusy.value = true
  try {
    const r = await External.importProblem(extSource.value, extID.value.trim(), 'members')
    toast.success(`已导入 #${r.problem.id}（外部题面，无测试数据，请补充后使用）`)
    extVisible.value = false
    extID.value = ''
    extPreview.value = null
    router.push(`/problems/${r.problem.id}/edit`)
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    extBusy.value = false
  }
}
</script>

<template>
  <Card>
    <CardContent class="p-6">
      <div class="mb-3 flex flex-wrap items-center gap-3">
        <Button @click="openCreate">新建题目</Button>
        <span class="h-6 w-px bg-border" />
        <input
          type="file"
          accept=".zip"
          class="text-sm text-muted-foreground"
          @change="(e: Event) => importFile = (e.target as HTMLInputElement).files?.[0] ?? null"
        />
        <Button variant="outline" :disabled="importing" @click="importPackage">
          {{ importing ? '导入中…' : '导入题目包' }}
        </Button>
        <Button variant="outline" @click="openExternal">外部题面导入</Button>
        <span class="text-xs text-muted-foreground">支持自有导出 zip / DOMjudge 题包 / Hydro 题包；外部导入仅题面+样例</span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-20">#</TableHead>
            <TableHead>标题</TableHead>
            <TableHead class="w-28">可见性</TableHead>
            <TableHead class="w-28">类型</TableHead>
            <TableHead class="w-30">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in items" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.title }}</TableCell>
            <TableCell>{{ row.visibility }}</TableCell>
            <TableCell>{{ row.judge_mode }}</TableCell>
            <TableCell>
              <Button size="sm" variant="outline" @click="openEdit(row)">编辑</Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Pagination class="mt-3" :page="page" :page-size="20" :total="total" @update:page="onPage" />

      <Dialog v-model:open="extVisible">
        <DialogContent class="max-w-[720px]">
          <DialogHeader>
            <DialogTitle>外部题面导入（插件爬虫）</DialogTitle>
          </DialogHeader>
          <div class="mb-3 flex gap-2">
            <Select v-model="extSource">
              <SelectTrigger class="w-40">
                <SelectValue placeholder="选择来源" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="s in extSources" :key="s" :value="s">{{ s }}</SelectItem>
              </SelectContent>
            </Select>
            <Input
              v-model="extID"
              class="flex-1"
              placeholder="题号：CF 1900A / 洛谷 P1001"
              @keyup.enter="extPreviewRun"
            />
            <Button :disabled="extBusy" @click="extPreviewRun">
              {{ extBusy ? '处理中…' : '预览' }}
            </Button>
          </div>
          <Alert variant="warning" class="mb-3"
            title="外部导入仅含题面与公开样例，无完整测试数据；默认仅登录可见，补齐数据后才可用于正式评测。" />
          <div v-if="extPreview" class="max-h-80 overflow-auto rounded-md border border-border p-3">
            <p><b>{{ extPreview.title }}</b></p>
            <p class="text-xs text-muted-foreground">{{ extPreview.url }}</p>
            <pre v-if="extPreview.statement_md" class="whitespace-pre-wrap text-xs">{{ extPreview.statement_md.slice(0, 1500) }}</pre>
            <p v-else class="text-sm text-wa">⚠ 该平台未返回题面内容（可能反爬或题面为图形/公式渲染），请打开原题链接人工补充题面。</p>
            <p v-for="(n, i) in extPreview.notes" :key="i" class="text-xs text-tle">⚠ {{ n }}</p>
          </div>
          <DialogFooter>
            <Button variant="outline" @click="extVisible = false">取消</Button>
            <Button :disabled="!extPreview || extBusy" @click="extImport">
              {{ extBusy ? '导入中…' : '确认导入' }}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </CardContent>
  </Card>
</template>
