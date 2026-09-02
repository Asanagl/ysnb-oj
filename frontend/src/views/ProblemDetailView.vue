<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Problems,
  Solutions,
  connectWS,
  errMsg,
  type ProblemSolution,
} from '../api/client'
import { useAuthStore } from '../stores/auth'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import CodeEditor from '../components/CodeEditor.vue'
import StatusTag from '../components/StatusTag.vue'

const route = useRoute()
const auth = useAuthStore()
const problem = ref<Awaited<ReturnType<typeof Problems.get>> | null>(null)
const solutions = ref<ProblemSolution[]>([])
const languages = ref<{ id: string; name: string }[]>([])
const lang = ref('cpp')
const code = ref('')
const submitting = ref(false)
const lastSub = ref<{ id: number; status: string; cases: { index: number; status: string; time_ms: number; mem_kb: number; message?: string }[]; compile_message?: string } | null>(null)
let ws: ReturnType<typeof connectWS> | null = null

const statementHTML = computed(() =>
  problem.value ? renderStatement(problem.value.problem.statement_md) : '',
)
const samples = computed(() => problem.value?.samples ?? [])

// 题解区 state
// 题解区默认隐藏：任意提交解锁；比赛进行中全锁（赛后开放）；
// 作者/管理员/裁判豁免。后端 403 时展示锁定态而不是报错。
const solLocked = ref<null | string>(null)
const editorOpen = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const solForm = ref({ id: 0, title: '', body_html: '', parent_id: null as number | null, is_official: false })
const replyTo = ref<ProblemSolution | null>(null)

// 所见即所得编辑的 HTML 直接作为 body_md 存储；渲染端按 HTML 输出
const isModerator = computed(() => {
  if (!problem.value || !auth.user) return false
  return auth.isAdmin || problem.value.problem.created_by === auth.user.id
})

onMounted(async () => {
  problem.value = await Problems.get(route.params.id as string)
  try {
    solutions.value = await Solutions.list(route.params.id as string)
  } catch (e: unknown) {
    // 403 = locked: record the backend reason for the locked-state panel
    solLocked.value = errMsg(e)
  }
  const r = await fetch('/api/v1/languages', {
    headers: { Authorization: `Bearer ${localStorage.getItem('oj_token')}` },
  })
  languages.value = (await r.json()) as { id: string; name: string }[]
  ws = connectWS([], () => {})
})
onBeforeUnmount(() => ws?.close())

function openCreate(parent?: ProblemSolution) {
  replyTo.value = parent ?? null
  editorMode.value = 'create'
  solForm.value = { id: 0, title: parent ? `回复：${parent.username}` : '', body_html: '', parent_id: parent?.id ?? null, is_official: false }
  editorOpen.value = true
}

function openEdit(sol: ProblemSolution) {
  editorMode.value = 'edit'
  solForm.value = { id: sol.id, title: sol.title, body_html: sol.body_md, parent_id: sol.parent_id, is_official: sol.is_official }
  editorOpen.value = true
}

async function saveSolution() {
  if (!solForm.value.body_html.trim()) {
    ElMessage.warning('内容不能为空')
    return
  }
  try {
    if (editorMode.value === 'edit') {
      await Solutions.update(route.params.id as string, solForm.value.id, {
        title: solForm.value.title,
        body_md: solForm.value.body_html,
        is_official: solForm.value.is_official,
      })
    } else {
      await Solutions.create(route.params.id as string, {
        title: solForm.value.title,
        body_md: solForm.value.body_html,
        parent_id: solForm.value.parent_id ?? undefined,
      })
    }
    solutions.value = await Solutions.list(route.params.id as string)
    editorOpen.value = false
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function toggleOfficial(sol: ProblemSolution) {
  try {
    await Solutions.update(route.params.id as string, sol.id, {
      title: sol.title, body_md: sol.body_md, is_official: !sol.is_official,
    })
    solutions.value = await Solutions.list(route.params.id as string)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function deleteSolution(sol: ProblemSolution) {
  try {
    await Solutions.remove(route.params.id as string, sol.id)
    solutions.value = await Solutions.list(route.params.id as string)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function submit() {
  if (!problem.value) return
  submitting.value = true
  try {
    const sub = await Submissions.create({
      problem_id: problem.value.problem.id,
      language: lang.value,
      code: code.value,
    })
    lastSub.value = sub
    ElMessage.success('已提交，等待判题')
    pollSubmission(sub.id)
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    submitting.value = false
  }
}

async function pollSubmission(id: number) {
  for (let i = 0; i < 120; i++) {
    await new Promise((r) => setTimeout(r, 1000))
    try {
      const s = await Submissions.get(id)
      lastSub.value = s
      if (!['PENDING', 'COMPILING', 'JUDGING'].includes(s.status)) return
    } catch {
      return
    }
  }
}

import { Submissions } from '../api/client'

const topLevel = computed(() => solutions.value.filter((s) => !s.parent_id))
const repliesOf = (id: number) => solutions.value.filter((s) => s.parent_id === id)
const canEditSolution = (sol: ProblemSolution) =>
  auth.user && (sol.user_id === auth.user.id || isModerator.value)
</script>

<template>
  <div v-if="problem">
    <el-row :gutter="16">
      <el-col :span="13">
        <el-card>
          <h2>{{ problem.problem.title }}</h2>
          <p style="color: #909399">
            时限 {{ problem.problem.time_limit_ms }} ms · 内存 {{ problem.problem.mem_limit_mb }} MB
            · 测试点 {{ problem.case_count }}
          </p>
          <div class="statement" v-html="statementHTML" />
          <template v-if="samples.length">
            <h4>样例</h4>
            <div v-for="(s, i) in samples" :key="i" class="sample-pair">
              <el-input type="textarea" :model-value="s.input" readonly :rows="3" placeholder="输入" />
              <el-input type="textarea" :model-value="s.output" readonly :rows="3" placeholder="输出" />
            </div>
          </template>
          <div v-if="problem.problem.hint">
            <h4>提示</h4>
            <div class="statement" v-html="renderStatement(problem.problem.hint)" />
          </div>
        </el-card>
      </el-col>
      <el-col :span="11">
        <el-card>
          <h4>提交代码</h4>
          <el-select v-model="lang" style="margin-bottom: 8px; width: 220px">
            <el-option v-for="l in languages" :key="l.id" :label="l.name" :value="l.id" />
          </el-select>
          <CodeEditor v-model="code" :language="lang" />
          <el-button type="primary" style="margin-top: 12px" :loading="submitting" @click="submit">
            提交
          </el-button>
        </el-card>
        <el-card v-if="lastSub" style="margin-top: 12px">
          <h4>
            最近提交 #{{ lastSub.id }}
            <StatusTag :status="lastSub.status" />
          </h4>
          <p v-if="lastSub.compile_message" style="white-space: pre-wrap; color: #c0c4cc">
            {{ lastSub.compile_message }}
          </p>
          <el-table v-if="lastSub.cases.length" :data="lastSub.cases" size="small">
            <el-table-column label="#" prop="index" width="60" />
            <el-table-column label="状态">
              <template #default="{ row }"><StatusTag :status="row.status" /></template>
            </el-table-column>
            <el-table-column label="耗时" prop="time_ms" width="90" />
            <el-table-column label="内存" prop="mem_kb" width="90" />
            <el-table-column label="信息" prop="message" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-card style="margin-top: 16px">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
        <h3 style="margin: 0">题解区<template v-if="!solLocked">（{{ solutions.length }}）</template></h3>
        <el-button v-if="!solLocked" type="primary" @click="openCreate()">写题解</el-button>
      </div>

      <el-empty v-if="solLocked" :description="solLocked">
        <template #image><span style="font-size: 42px">🔒</span></template>
        <p style="color: #909399; font-size: 13px; margin: 0">
          提交一次本题（任意结果）即可解锁题解区；比赛题目将在比赛结束后开放。
        </p>
      </el-empty>

      <template v-else>
      <div v-for="sol in topLevel" :key="sol.id" class="sol-floor" :class="{ official: sol.is_official }">
        <div class="sol-head">
          <el-tag v-if="sol.is_official" type="success" size="small">官方题解</el-tag>
          <router-link :to="`/users/${sol.user_id}`" class="sol-user">{{ sol.username }}</router-link>
          <span style="color: #c0c4cc; font-size: 12px">{{ new Date(sol.created_at).toLocaleString() }}</span>
          <span style="flex: 1" />
          <el-button v-if="canEditSolution(sol)" size="small" text @click="openEdit(sol)">编辑</el-button>
          <el-button v-if="isModerator" size="small" :text="true" @click="toggleOfficial(sol)">
            {{ sol.is_official ? '取消置顶' : '设为官方' }}
          </el-button>
          <el-button v-if="canEditSolution(sol)" size="small" type="danger" text @click="deleteSolution(sol)">删除</el-button>
        </div>
        <div v-if="sol.title" class="sol-title">{{ sol.title }}</div>
        <div class="sol-body wysiwyg-render" v-html="sol.body_md" />
        <div style="margin-top: 6px">
          <el-button size="small" text @click="openCreate(sol)">回复</el-button>
        </div>
        <div v-for="r in repliesOf(sol.id)" :key="r.id" class="sol-reply">
          <div class="sol-head">
            <router-link :to="`/users/${r.user_id}`" class="sol-user">{{ r.username }}</router-link>
            <span style="color: #c0c4cc; font-size: 12px">{{ new Date(r.created_at).toLocaleString() }}</span>
            <span style="flex: 1" />
            <el-button v-if="canEditSolution(r)" size="small" text @click="openEdit(r)">编辑</el-button>
            <el-button v-if="canEditSolution(r)" size="small" type="danger" text @click="deleteSolution(r)">删除</el-button>
          </div>
          <div class="sol-body wysiwyg-render" v-html="r.body_md" />
        </div>
      </div>
      <el-empty v-if="topLevel.length === 0" description="还没有题解，来写第一篇吧" />

      </template>

      <el-dialog
        v-model="editorOpen"
        :title="editorMode === 'edit' ? '编辑题解' : replyTo ? `回复 @${replyTo.username}` : '写题解'"
        width="72%"
      >
        <el-input v-if="!replyTo" v-model="solForm.title" placeholder="标题（可选）" style="margin-bottom: 10px" />
        <MarkdownEditor v-model="solForm.body_html" placeholder="书写题解内容，支持加粗/标题/代码块/图片/链接…" />
        <template #footer>
          <el-button @click="editorOpen = false">取消</el-button>
          <el-button type="primary" @click="saveSolution">保存</el-button>
        </template>
      </el-dialog>
    </el-card>
  </div>
</template>

<style scoped>
.sample-pair {
  display: flex;
  gap: 12px;
}
@media (max-width: 767.98px) {
  .sample-pair {
    display: block;
  }
  .sample-pair .el-textarea {
    margin-bottom: 8px;
  }
}
.sol-floor {
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}
.sol-floor.official {
  border-color: #67c23a;
  background: rgba(103, 194, 58, 0.04);
}
.sol-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.sol-user {
  font-weight: 600;
  color: #303133;
  text-decoration: none;
}
.sol-user:hover {
  color: #409eff;
}
.sol-title {
  font-weight: 600;
  margin-bottom: 6px;
}
.sol-body {
  line-height: 1.75;
}
.sol-reply {
  border-left: 3px solid #ebeef5;
  padding: 8px 0 8px 14px;
  margin: 10px 0 0 24px;
}
.wysiwyg-render :deep(p) {
  margin: 6px 0;
}
.wysiwyg-render :deep(pre) {
  background: #282c34;
  color: #abb2bf;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}
.wysiwyg-render :deep(img) {
  max-width: 100%;
}
.wysiwyg-render :deep(blockquote) {
  border-left: 3px solid #409eff;
  padding-left: 12px;
  color: #606266;
  margin-left: 0;
}
</style>
