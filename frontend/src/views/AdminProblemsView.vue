<script setup lang="ts">
// 题目管理列表：新建/编辑跳转到独立全页编辑器 ProblemEditorView。
// 「导入题目包」就地解析 zip（自有格式 / DOMjudge / Hydro），导入后跳转编辑。
// 「外部题面导入」走插件爬虫（CF/洛谷），仅题面+公开样例，无测试数据。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { errMsg, External, Problems } from '../api/client'

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

function openCreate() {
  router.push('/problems/new')
}

function openEdit(row: { id: number }) {
  router.push(`/problems/${row.id}/edit`)
}

async function importPackage() {
  if (!importFile.value) {
    ElMessage.warning('请先选择题目包 zip')
    return
  }
  importing.value = true
  try {
    const r = await Problems.importPackage(importFile.value)
    const notes = r.notes?.length ? `；${r.notes.join('；')}` : ''
    ElMessage.success(`已导入 #${r.problem.id}（${r.stored} 个测试点，格式 ${r.format}）${notes}`)
    router.push(`/problems/${r.problem.id}/edit`)
  } catch (e) {
    ElMessage.error(errMsg(e))
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
async function extPreviewRun() {
  if (!extID.value.trim()) {
    ElMessage.warning('请填写外部题号（如 1900A / P1001）')
    return
  }
  extBusy.value = true
  extPreview.value = null
  try {
    const r = await External.previewProblem(extSource.value, extID.value.trim())
    extPreview.value = {
      title: r.meta.title, url: r.meta.url,
      statement_md: r.meta.statement_md, notes: r.meta.notes ?? [],
    }
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    extBusy.value = false
  }
}

async function extImport() {
  extBusy.value = true
  try {
    const r = await External.importProblem(extSource.value, extID.value.trim(), 'members')
    ElMessage.success(`已导入 #${r.problem.id}（外部题面，无测试数据，请补充后使用）`)
    extVisible.value = false
    extID.value = ''
    extPreview.value = null
    router.push(`/problems/${r.problem.id}/edit`)
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    extBusy.value = false
  }
}
</script>

<template>
  <el-card>
    <div style="margin-bottom: 12px; display: flex; gap: 12px; align-items: center; flex-wrap: wrap">
      <el-button type="primary" @click="openCreate">新建题目</el-button>
      <el-divider direction="vertical" />
      <input type="file" accept=".zip" @change="(e: Event) => importFile = (e.target as HTMLInputElement).files?.[0] ?? null" />
      <el-button type="success" plain :loading="importing" @click="importPackage">导入题目包</el-button>
      <el-button type="warning" plain @click="openExternal">外部题面导入</el-button>
      <span style="color: #909399; font-size: 12px">支持自有导出 zip / DOMjudge 题包 / Hydro 题包；外部导入仅题面+样例</span>
    </div>
    <el-table :data="items">
      <el-table-column label="#" prop="id" width="80" />
      <el-table-column label="标题" prop="title" />
      <el-table-column label="可见性" prop="visibility" width="110" />
      <el-table-column label="类型" prop="judge_mode" width="110" />
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-model:current-page="page"
      layout="prev, pager, next"
      :total="total"
      :page-size="20"
      style="margin-top: 12px"
      @current-change="load"
    />

    <el-dialog v-model="extVisible" title="外部题面导入（插件爬虫）" width="720px">
      <div style="display: flex; gap: 8px; margin-bottom: 12px">
        <el-select v-model="extSource" style="width: 160px">
          <el-option v-for="s in extSources" :key="s" :label="s" :value="s" />
        </el-select>
        <el-input v-model="extID" placeholder="题号：CF 1900A / 洛谷 P1001" @keyup.enter="extPreviewRun" />
        <el-button :loading="extBusy" @click="extPreviewRun">预览</el-button>
      </div>
      <el-alert type="warning" :closable="false" style="margin-bottom: 12px"
        title="外部导入仅含题面与公开样例，无完整测试数据；默认仅登录可见，补齐数据后才可用于正式评测。" />
      <div v-if="extPreview" style="border: 1px solid #ebeef5; border-radius: 6px; padding: 12px; max-height: 320px; overflow: auto">
        <p><b>{{ extPreview.title }}</b></p>
        <p style="color: #909399; font-size: 12px">{{ extPreview.url }}</p>
        <pre style="white-space: pre-wrap; font-size: 12px">{{ extPreview.statement_md.slice(0, 1500) }}</pre>
        <p v-for="(n, i) in extPreview.notes" :key="i" style="color: #e6a23c; font-size: 12px">⚠ {{ n }}</p>
      </div>
      <template #footer>
        <el-button @click="extVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!extPreview" :loading="extBusy" @click="extImport">确认导入</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>