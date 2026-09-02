<script setup lang="ts">
// 独立全页出题编辑器：左侧编辑、右侧实时预览，整页可用宽度。
// AdminProblemsView / MyProblemsView 的「新建/编辑」都跳到这里。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { errMsg, Problems } from '../api/client'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'

const route = useRoute()
const router = useRouter()
const problemId = computed(() => (route.params.id ? Number(route.params.id) : 0))
const loading = ref(false)
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
  loading.value = true
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
    ElMessage.error(errMsg(e))
  } finally {
    loading.value = false
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
    ElMessage.warning('标题不能为空')
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
      ElMessage.success('已保存')
    } else {
      const created = await Problems.create(payload)
      ElMessage.success(`已创建题目 #${created.id}`)
      // replace the route so further saves are edits on the real problem
      router.replace(`/problems/${created.id}/edit`)
    }
    if (!stay) router.back()
  } catch (e) {
    ElMessage.error(errMsg(e))
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
    ElMessage.success(`已导入 ${r.stored} 个测试点`)
    await loadTestdataRows()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function uploadCase() {
  if (!caseInputFile.value || !problemId.value) {
    ElMessage.warning('选择输入文件')
    return
  }
  try {
    await Problems.uploadCase(problemId.value, caseInputFile.value, caseOutputFile.value ?? undefined)
    ElMessage.success('测试点已添加')
    caseInputFile.value = null
    caseOutputFile.value = null
    await loadTestdataRows()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function previewCase(caseId: number, kind: string) {
  try {
    previewText.value = await Problems.previewCase(problemId.value, caseId, kind)
    previewKind.value = `${caseId}.${kind}`
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function deleteCase(caseId: number) {
  try {
    await Problems.deleteCase(problemId.value, caseId)
    ElMessage.success('已删除')
    await loadTestdataRows()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function copyProblem() {
  if (!problemId.value) return
  try {
    const copy = await Problems.copy(problemId.value)
    ElMessage.success(`已复制为新题目 #${copy.id}`)
    router.push(`/problems/${copy.id}/edit`)
  } catch (e) {
    ElMessage.error(errMsg(e))
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
    ElMessage.warning('请先选择 .cpp 文件')
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
    ElMessage.success(`${kind === 'checker' ? 'Checker' : 'Interactor'} 源码已上传并保存`)
  } catch (e) {
    ElMessage.error(errMsg(e))
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
  <div v-loading="loading" class="pe-root">
    <div class="pe-header">
      <h3 style="margin: 0">
        {{ problemId ? `编辑题目 #${problemId}` : '新建题目' }}
      </h3>
      <span style="flex: 1" />
      <el-button @click="router.back()">返回</el-button>
      <el-button type="primary" :loading="saving" @click="save()">保存</el-button>
    </div>

    <div class="pe-columns">
      <!-- 左：编辑 -->
      <el-card class="pe-edit" shadow="never">
        <el-form label-position="top">
          <el-form-item label="标题">
            <el-input v-model="form.title" placeholder="题目名称" />
          </el-form-item>
          <el-form-item label="题面（所见即所得，支持代码块/图片/链接；右侧实时预览）">
            <MarkdownEditor
              v-model="form.statement_md"
              placeholder="输入题面内容…"
              class="pe-statement-editor"
            />
          </el-form-item>
          <el-form-item label="输入描述">
            <el-input v-model="form.input_desc" type="textarea" :rows="3" />
          </el-form-item>
          <el-form-item label="输出描述">
            <el-input v-model="form.output_desc" type="textarea" :rows="3" />
          </el-form-item>
          <el-form-item label="提示（可选）">
            <el-input v-model="form.hint" type="textarea" :rows="3" />
          </el-form-item>

          <el-row :gutter="12">
            <el-col :span="6">
              <el-form-item label="时限 ms">
                <el-input-number v-model="form.time_limit_ms" :min="100" :step="100" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="内存 MB">
                <el-input-number v-model="form.mem_limit_mb" :min="16" :step="16" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="可见性">
                <el-select v-model="form.visibility">
                  <el-option label="隐藏（仅出题人）" value="hidden" />
                  <el-option label="仅注册用户" value="members" />
                  <el-option label="公开" value="public" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="判题模式">
                <el-select v-model="form.judge_mode">
                  <el-option label="标准比对" value="default" />
                  <el-option label="SPJ 特判" value="spj" />
                  <el-option label="交互题" value="interactive" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="标签（JSON 数组或逗号分隔）">
            <el-input v-model="form.tags" placeholder='["dp","greedy"] 或 dp, greedy' />
          </el-form-item>
          <el-form-item label="来源">
            <el-input v-model="form.source" />
          </el-form-item>

          <el-form-item label="样例">
            <div style="width: 100%">
              <div v-for="(s, i) in samplesParsed" :key="i" class="pe-sample">
                <el-input v-model="s.input" type="textarea" :rows="2" placeholder="样例输入" />
                <el-input v-model="s.output" type="textarea" :rows="2" placeholder="样例输出" />
                <el-button type="danger" text @click="removeSample(i)">删除</el-button>
              </div>
              <el-button size="small" @click="addSample">+ 添加样例</el-button>
            </div>
          </el-form-item>

          <el-form-item v-if="form.judge_mode === 'spj'" label="Checker 源码（C++17 testlib 风格）">
            <div style="width: 100%">
              <el-input
                v-model="form.checker_source"
                type="textarea"
                :rows="10"
                placeholder="./checker <input> <answer> <output>，exit 0 = AC"
              />
              <div style="display: flex; gap: 8px; align-items: center; margin-top: 6px">
                <input type="file" accept=".cpp,.cc,.cxx" @change="(e: Event) => checkerFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <el-button size="small" :disabled="!problemId" @click="uploadJudgeSource('checker')">上传并保存</el-button>
              </div>
            </div>
          </el-form-item>
          <el-form-item v-if="form.judge_mode === 'interactive'" label="Interactor 源码（C++17）">
            <div style="width: 100%">
              <el-input
                v-model="form.interactor_source"
                type="textarea"
                :rows="10"
                placeholder="./interactor <input>，stdin/stdout 与选手程序直连，exit 0 = AC / 1 = WA"
              />
              <div style="display: flex; gap: 8px; align-items: center; margin-top: 6px">
                <input type="file" accept=".cpp,.cc,.cxx" @change="(e: Event) => interactorFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <el-button size="small" :disabled="!problemId" @click="uploadJudgeSource('interactor')">上传并保存</el-button>
              </div>
            </div>
          </el-form-item>

          <el-form-item v-if="problemId" label="测试数据">
            <div style="width: 100%">
              <div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 8px">
                <input type="file" accept=".zip" @change="(e: Event) => uploadFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <el-button size="small" type="primary" @click="onUpload">整包 zip 替换</el-button>
                <el-divider direction="vertical" />
                <input type="file" accept=".in,.txt" @change="(e: Event) => caseInputFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <input type="file" accept=".out,.txt" @change="(e: Event) => caseOutputFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
                <el-button size="small" @click="uploadCase">追加单个测试点</el-button>
                <el-button size="small" @click="copyProblem">复制题目</el-button>
                <el-button size="small" @click="exportProblem">导出 zip</el-button>
                <el-divider direction="vertical" />
                <span v-if="!problemId" style="color: #909399; font-size: 12px">保存后可导入题目包</span>
              </div>
              <el-table v-if="testdataRows.length" :data="testdataRows" size="small">
                <el-table-column label="测试点" prop="index" width="70" />
                <el-table-column label="输入字节" prop="input_size" width="90" />
                <el-table-column label="答案字节" prop="answer_size" width="90" />
                <el-table-column label="预览" width="160">
                  <template #default="{ row }">
                    <el-button size="small" text @click="previewCase(row.id, 'in')">in</el-button>
                    <el-button v-if="row.answer_size" size="small" text @click="previewCase(row.id, 'out')">out</el-button>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="80">
                  <template #default="{ row }">
                    <el-popconfirm title="删除该测试点？" @confirm="deleteCase(row.id)">
                      <template #reference>
                        <el-button size="small" type="danger" text>删除</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </el-form-item>
          <el-alert v-else title="先保存题目后才能上传测试数据" type="warning" :closable="false" />
        </el-form>
      </el-card>

      <!-- 右：实时预览 -->
      <el-card class="pe-preview" shadow="never">
        <h4 style="margin-top: 0">实时预览</h4>
        <h2 style="margin-top: 0">{{ form.title || '（无标题）' }}</h2>
        <div class="pe-section" v-html="statementHTML" />
        <template v-if="form.input_desc || form.output_desc">
          <h4>输入格式</h4>
          <div class="pe-section" v-html="renderStatement(form.input_desc)" />
          <h4>输出格式</h4>
          <div class="pe-section" v-html="renderStatement(form.output_desc)" />
        </template>
        <template v-for="(s, i) in samplesParsed" :key="i">
          <h4>样例 {{ i + 1 }}</h4>
          <div class="pe-sample-view">
            <pre class="pe-io">{{ s.input }}</pre>
            <pre class="pe-io">{{ s.output }}</pre>
          </div>
        </template>
        <template v-if="form.hint">
          <h4>提示</h4>
          <div class="pe-section" v-html="renderStatement(form.hint)" />
        </template>
      </el-card>
    </div>

    <el-dialog :model-value="previewText !== ''" :title="previewKind" width="60%" @update:model-value="previewText = ''">
      <pre style="white-space: pre-wrap; max-height: 400px; overflow: auto">{{ previewText }}</pre>
    </el-dialog>
  </div>
</template>

<style scoped>
.pe-root {
  max-width: 1800px;
  margin: 0 auto;
}
.pe-header {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
}
.pe-columns {
  display: grid;
  grid-template-columns: minmax(0, 7fr) minmax(0, 5fr);
  gap: 16px;
  align-items: start;
}
.pe-edit {
  min-width: 0;
}
.pe-statement-editor {
  width: 100%;
}
.pe-statement-editor :deep(.wysiwyg-content) {
  min-height: 320px;
}
.pe-preview {
  position: sticky;
  top: 16px;
  max-height: calc(100vh - 90px);
  overflow: auto;
}
.pe-sample {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 8px;
  align-items: start;
  margin-bottom: 8px;
  width: 100%;
}
.pe-sample-view {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.pe-io {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 8px;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  min-height: 2em;
}
@media (max-width: 1279.98px) {
  .pe-columns {
    grid-template-columns: 1fr;
  }
  .pe-preview {
    position: static;
    max-height: none;
    order: -1;
  }
  .pe-sample {
    grid-template-columns: 1fr;
  }
}
</style>
