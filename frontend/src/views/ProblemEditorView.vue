<script setup lang="ts">
// 独立全页出题编辑器：左侧编辑、右侧实时预览，整页可用宽度。
// AdminProblemsView / MyProblemsView 的「新建/编辑」都跳到这里。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { errMsg, Problems } from '../api/client'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NumberInput } from '@/components/ui/number-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FormField } from '@/components/ui/form-field'
import { Alert } from '@/components/ui/alert'
import { Separator } from '@/components/ui/separator'
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
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'

const route = useRoute()
const router = useRouter()
const problemId = computed(() => (route.params.id ? Number(route.params.id) : 0))
const saving = ref(false)
const testdataRows = ref<{ id: number; index: number; input_size: number; answer_size: number }[]>([])
const uploadFile = ref<File | null>(null)
const caseInputFile = ref<File | null>(null)
const caseOutputFile = ref<File | null>(null)
const previewText = ref('')
const previewKind = ref('')

const form = reactive({
  title: '',
  statement_md: '',
  input_desc: '',
  output_desc: '',
  hint: '',
  source: '',
  tags: '',
  samples: '[]',
  time_limit_ms: 1000,
  mem_limit_mb: 256,
  visibility: 'members',
  judge_mode: 'default',
  checker_source: '',
  interactor_source: '',
})

const statementHTML = computed(() => renderStatement(form.statement_md))
const samplesParsed = computed<{ input: string; output: string }[]>(() => {
  try {
    return JSON.parse(form.samples || '[]')
  } catch {
    return []
  }
})

onMounted(async () => {
  if (!problemId.value) return
  try {
    const detail = await Problems.get(problemId.value)
    const p = detail.problem
    Object.assign(form, {
      title: p.title, statement_md: p.statement_md,
      input_desc: p.input_desc, output_desc: p.output_desc, hint: p.hint,
      source: p.source, tags: p.tags, samples: p.samples,
      time_limit_ms: p.time_limit_ms, mem_limit_mb: p.mem_limit_mb,
      visibility: p.visibility, judge_mode: p.judge_mode,
      checker_source: detail.checker_source ?? '',
      interactor_source: detail.interactor_source ?? '',
    })
    testdataRows.value = await Problems.testdata(p.id)
  } catch (e) {
    toast.error(errMsg(e))
  }
})

function parseTags(): string[] {
  try {
    return JSON.parse(form.tags || '[]') as string[]
  } catch {
    return form.tags.split(',').map((s) => s.trim()).filter(Boolean)
  }
}

async function save(stay = true) {
  if (!form.title.trim()) {
    toast.warning('标题不能为空')
    return
  }
  saving.value = true
  try {
    const payload = {
      title: form.title,
      statement_md: form.statement_md,
      input_desc: form.input_desc,
      output_desc: form.output_desc,
      hint: form.hint,
      source: form.source,
      tags: parseTags(),
      samples: JSON.parse(form.samples || '[]'),
      time_limit_ms: form.time_limit_ms,
      mem_limit_mb: form.mem_limit_mb,
      visibility: form.visibility,
      judge_mode: form.judge_mode,
      checker_source: form.judge_mode === 'spj' ? form.checker_source : '',
      interactor_source: form.judge_mode === 'interactive' ? form.interactor_source : '',
    }
    if (problemId.value) {
      await Problems.update(problemId.value, payload)
      toast.success('已保存')
    } else {
      const created = await Problems.create(payload)
      toast.success(`已创建题目 #${created.id}`)
      // replace the route so further saves are edits on the real problem
      router.replace(`/problems/${created.id}/edit`)
    }
    if (!stay) router.back()
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

async function loadTestdataRows() {
  if (!problemId.value) return
  testdataRows.value = await Problems.testdata(problemId.value)
}

async function onUpload() {
  if (!uploadFile.value || !problemId.value) return
  try {
    const r = await Problems.uploadTestdata(problemId.value, uploadFile.value)
    toast.success(`已导入 ${r.stored} 个测试点`)
    await loadTestdataRows()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function uploadCase() {
  if (!caseInputFile.value || !problemId.value) {
    toast.warning('选择输入文件')
    return
  }
  try {
    await Problems.uploadCase(problemId.value, caseInputFile.value, caseOutputFile.value ?? undefined)
    toast.success('测试点已添加')
    caseInputFile.value = null
    caseOutputFile.value = null
    await loadTestdataRows()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function previewCase(caseId: number, kind: string) {
  try {
    previewText.value = await Problems.previewCase(problemId.value, caseId, kind)
    previewKind.value = `${caseId}.${kind}`
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function deleteCase(caseId: number) {
  if (!(await confirmDialog({ title: '删除该测试点？', danger: true }))) return
  try {
    await Problems.deleteCase(problemId.value, caseId)
    toast.success('已删除')
    await loadTestdataRows()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function copyProblem() {
  if (!problemId.value) return
  try {
    const copy = await Problems.copy(problemId.value)
    toast.success(`已复制为新题目 #${copy.id}`)
    router.push(`/problems/${copy.id}/edit`)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

function exportProblem() {
  if (!problemId.value) return
  window.location.href = `/api/v1/problems/${problemId.value}/export`
}

// checker/interactor 上传：文件内容直接覆盖文本框（保存时随表单提交），
// 与「贴源码」走同一条数据路径，避免两套来源不一致。
const checkerFile = ref<File | null>(null)
const interactorFile = ref<File | null>(null)

async function uploadJudgeSource(kind: 'checker' | 'interactor') {
  if (!problemId.value) return
  const file = kind === 'checker' ? checkerFile.value : interactorFile.value
  if (!file) {
    toast.warning('请先选择 .cpp 文件')
    return
  }
  try {
    await Problems.uploadJudgeSource(problemId.value, kind, file)
    const text = await file.text()
    if (kind === 'checker') {
      form.checker_source = text
      checkerFile.value = null
    } else {
      form.interactor_source = text
      interactorFile.value = null
    }
    toast.success(`${kind === 'checker' ? 'Checker' : 'Interactor'} 源码已上传并保存`)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

function addSample() {
  const arr = samplesParsed.value
  arr.push({ input: '', output: '' })
  form.samples = JSON.stringify(arr)
}

function removeSample(i: number) {
  const arr = samplesParsed.value
  arr.splice(i, 1)
  form.samples = JSON.stringify(arr)
}
</script>

<template>
  <div class="mx-auto max-w-[1800px]">
    <div class="mb-3 flex items-center gap-3">
      <h3 class="m-0 text-lg font-semibold">
        {{ problemId ? `编辑题目 #${problemId}` : '新建题目' }}
      </h3>
      <span class="flex-1" />
      <Button variant="outline" @click="router.back()">返回</Button>
      <Button :disabled="saving" @click="save()">{{ saving ? '保存中…' : '保存' }}</Button>
    </div>

    <div class="grid items-start gap-4 xl:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
      <!-- 左：编辑 -->
      <Card class="min-w-0">
        <CardContent class="space-y-4 p-6">
          <FormField label="标题">
            <Input v-model="form.title" placeholder="题目名称" />
          </FormField>
          <FormField label="题面（所见即所得，支持代码块/图片/链接；右侧实时预览）">
            <MarkdownEditor
              v-model="form.statement_md"
              placeholder="输入题面内容…"
              class="pe-statement-editor w-full"
            />
          </FormField>
          <FormField label="输入描述">
            <Textarea v-model="form.input_desc" :rows="3" />
          </FormField>
          <FormField label="输出描述">
            <Textarea v-model="form.output_desc" :rows="3" />
          </FormField>
          <FormField label="提示（可选）">
            <Textarea v-model="form.hint" :rows="3" />
          </FormField>

          <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
            <FormField label="时限 ms">
              <NumberInput v-model="form.time_limit_ms" :min="100" :step="100" />
            </FormField>
            <FormField label="内存 MB">
              <NumberInput v-model="form.mem_limit_mb" :min="16" :step="16" />
            </FormField>
            <FormField label="可见性">
              <Select v-model="form.visibility">
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="hidden">隐藏（仅出题人）</SelectItem>
                  <SelectItem value="members">仅注册用户</SelectItem>
                  <SelectItem value="public">公开</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label="判题模式">
              <Select v-model="form.judge_mode">
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default">标准比对</SelectItem>
                  <SelectItem value="spj">SPJ 特判</SelectItem>
                  <SelectItem value="interactive">交互题</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
          </div>

          <FormField label="标签（JSON 数组或逗号分隔）">
            <Input v-model="form.tags" placeholder='["dp","greedy"] 或 dp, greedy' />
          </FormField>
          <FormField label="来源">
            <Input v-model="form.source" />
          </FormField>

          <FormField label="样例">
            <div class="w-full">
              <div
                v-for="(s, i) in samplesParsed"
                :key="i"
                class="mb-2 grid w-full grid-cols-1 items-start gap-2 md:grid-cols-[1fr_1fr_auto]"
              >
                <Textarea v-model="s.input" :rows="2" placeholder="样例输入" />
                <Textarea v-model="s.output" :rows="2" placeholder="样例输出" />
                <Button variant="ghost" size="sm" class="text-destructive" @click="removeSample(i)">删除</Button>
              </div>
              <Button variant="outline" size="sm" @click="addSample">+ 添加样例</Button>
            </div>
          </FormField>

          <FormField v-if="form.judge_mode === 'spj'" label="Checker 源码（C++17 testlib 风格）">
            <div class="w-full">
              <Textarea
                v-model="form.checker_source"
                :rows="10"
                placeholder="./checker <input> <answer> <output>，exit 0 = AC"
              />
              <div class="mt-1.5 flex items-center gap-2">
                <input type="file" accept=".cpp,.cc,.cxx" class="text-sm text-muted-foreground" @change="(e: Event) => checkerFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <Button variant="outline" size="sm" :disabled="!problemId" @click="uploadJudgeSource('checker')">上传并保存</Button>
              </div>
            </div>
          </FormField>
          <FormField v-if="form.judge_mode === 'interactive'" label="Interactor 源码（C++17）">
            <div class="w-full">
              <Textarea
                v-model="form.interactor_source"
                :rows="10"
                placeholder="./interactor <input>，stdin/stdout 与选手程序直连，exit 0 = AC / 1 = WA"
              />
              <div class="mt-1.5 flex items-center gap-2">
                <input type="file" accept=".cpp,.cc,.cxx" class="text-sm text-muted-foreground" @change="(e: Event) => interactorFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <Button variant="outline" size="sm" :disabled="!problemId" @click="uploadJudgeSource('interactor')">上传并保存</Button>
              </div>
            </div>
          </FormField>

          <FormField v-if="problemId" label="测试数据">
            <div class="w-full">
              <div class="mb-2 flex flex-wrap items-center gap-2">
                <input type="file" accept=".zip" class="text-sm text-muted-foreground" @change="(e: Event) => uploadFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <Button size="sm" @click="onUpload">整包 zip 替换</Button>
                <Separator orientation="vertical" class="h-6" />
                <input type="file" accept=".in,.txt" class="text-sm text-muted-foreground" @change="(e: Event) => caseInputFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <input type="file" accept=".out,.txt" class="text-sm text-muted-foreground" @change="(e: Event) => caseOutputFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <Button variant="outline" size="sm" @click="uploadCase">追加单个测试点</Button>
                <Button variant="outline" size="sm" @click="copyProblem">复制题目</Button>
                <Button variant="outline" size="sm" @click="exportProblem">导出 zip</Button>
                <Separator orientation="vertical" class="h-6" />
                <span v-if="!problemId" class="text-xs text-muted-foreground">保存后可导入题目包</span>
              </div>
              <Table v-if="testdataRows.length">
                <TableHeader>
                  <TableRow>
                    <TableHead class="w-[70px]">测试点</TableHead>
                    <TableHead class="w-[90px]">输入字节</TableHead>
                    <TableHead class="w-[90px]">答案字节</TableHead>
                    <TableHead class="w-[160px]">预览</TableHead>
                    <TableHead class="w-[80px]">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="row in testdataRows" :key="row.id">
                    <TableCell>{{ row.index }}</TableCell>
                    <TableCell>{{ row.input_size }}</TableCell>
                    <TableCell>{{ row.answer_size }}</TableCell>
                    <TableCell>
                      <Button variant="ghost" size="sm" @click="previewCase(row.id, 'in')">in</Button>
                      <Button v-if="row.answer_size" variant="ghost" size="sm" @click="previewCase(row.id, 'out')">out</Button>
                    </TableCell>
                    <TableCell>
                      <Button variant="ghost" size="sm" class="text-destructive" @click="deleteCase(row.id)">删除</Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </FormField>
          <Alert v-else variant="warning" title="先保存题目后才能上传测试数据" />
        </CardContent>
      </Card>

      <!-- 右：实时预览 -->
      <Card class="order-first xl:order-last xl:sticky xl:top-4 xl:max-h-[calc(100vh-90px)] xl:overflow-auto">
        <CardContent class="p-6">
          <h4 class="mt-0">实时预览</h4>
          <h2 class="mt-0">{{ form.title || '（无标题）' }}</h2>
          <div class="pe-section" v-html="statementHTML" />
          <template v-if="form.input_desc || form.output_desc">
            <h4>输入格式</h4>
            <div class="pe-section" v-html="renderStatement(form.input_desc)" />
            <h4>输出格式</h4>
            <div class="pe-section" v-html="renderStatement(form.output_desc)" />
          </template>
          <template v-for="(s, i) in samplesParsed" :key="i">
            <h4>样例 {{ i + 1 }}</h4>
            <div class="grid grid-cols-2 gap-2">
              <pre class="m-0 min-h-[2em] whitespace-pre-wrap break-words rounded border border-border bg-muted p-2">{{ s.input }}</pre>
              <pre class="m-0 min-h-[2em] whitespace-pre-wrap break-words rounded border border-border bg-muted p-2">{{ s.output }}</pre>
            </div>
          </template>
          <template v-if="form.hint">
            <h4>提示</h4>
            <div class="pe-section" v-html="renderStatement(form.hint)" />
          </template>
        </CardContent>
      </Card>
    </div>

    <Dialog :open="previewText !== ''" @update:open="previewText = ''">
      <DialogContent class="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{{ previewKind }}</DialogTitle>
        </DialogHeader>
        <pre class="max-h-[400px] overflow-auto whitespace-pre-wrap">{{ previewText }}</pre>
      </DialogContent>
    </Dialog>
  </div>
</template>

<style scoped>
.pe-statement-editor :deep(.wysiwyg-content) {
  min-height: 320px;
}
</style>
