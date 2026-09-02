<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Contests,
  Problems,
  Submissions,
  errMsg,
  type Sample,
  type Submission,
} from '../api/client'
import { renderStatement } from '../utils/markdown'
import CodeEditor from '../components/CodeEditor.vue'
import StatusTag from '../components/StatusTag.vue'

const route = useRoute()
const router = useRouter()
const cid = Number(route.params.cid)
const contest = ref<Awaited<ReturnType<typeof Contests.get>> | null>(null)
const problem = ref<Awaited<ReturnType<typeof Problems.get>> | null>(null)
const languages = ref<{ id: string; name: string }[]>([])
const lang = ref('cpp')
const code = ref('')
const submitting = ref(false)
const lastSub = ref<Submission | null>(null)

const statementHTML = computed(() =>
  problem.value ? renderStatement(problem.value.problem.statement_md) : '',
)
const samples = computed<Sample[]>(() => problem.value?.samples ?? [])
const now = ref(Date.now())
const practice = computed(
  () => !!contest.value && Date.now() > new Date(contest.value.contest.end_time).getTime(),
)
const remaining = computed(() => {
  if (!contest.value) return ''
  const end = new Date(contest.value.contest.end_time).getTime()
  const ms = end - now.value
  if (ms <= 0) return '已结束 · 补题模式（不影响榜单）'
  const h = Math.floor(ms / 3600000)
  const m = Math.floor((ms % 3600000) / 60000)
  const s = Math.floor((ms % 60000) / 1000)
  return `剩余 ${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})
let ticker: number | undefined

onMounted(async () => {
  contest.value = await Contests.get(cid)
  problem.value = await Problems.get(route.params.pid as string)
  if (problem.value.problem.contest_id !== cid) {
    ElMessage.error('该题不属于当前比赛')
    router.push(`/contests/${cid}`)
    return
  }
  const r = await fetch('/api/v1/languages', {
    headers: { Authorization: `Bearer ${localStorage.getItem('oj_token')}` },
  })
  languages.value = (await r.json()) as { id: string; name: string }[]
  ticker = window.setInterval(() => (now.value = Date.now()), 1000)
})
onBeforeUnmount(() => clearInterval(ticker))

async function submit() {
  if (!problem.value) return
  submitting.value = true
  try {
    const sub = await Submissions.create({
      problem_id: problem.value.problem.id,
      language: lang.value,
      code: code.value,
      contest_id: cid,
    })
    lastSub.value = sub
    ElMessage.success(practice.value ? '补题已提交' : '已提交，等待判题')
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
</script>

<template>
  <div v-if="contest && problem">
    <el-card style="margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
        <el-button @click="router.push(`/contests/${cid}`)">返回比赛</el-button>
        <strong>{{ contest.contest.title }}</strong>
        <span style="color: #909399">{{ remaining }}</span>
        <el-tag v-if="practice" type="warning">补题</el-tag>
      </div>
    </el-card>

    <div class="pd-grid">
      <el-card class="pd-statement">
        <h2>{{ problem.problem.title }}</h2>
        <p style="color: #909399">
          时限 {{ problem.problem.time_limit_ms }} ms · 内存 {{ problem.problem.mem_limit_mb }} MB
        </p>
        <div class="statement" v-html="statementHTML" />
        <template v-if="samples.length">
          <h4>样例</h4>
          <div v-for="(s, i) in samples" :key="i" class="sample-pair">
            <el-input type="textarea" :model-value="s.input" readonly :rows="3" placeholder="输入" />
            <el-input type="textarea" :model-value="s.output" readonly :rows="3" placeholder="输出" />
          </div>
        </template>
      </el-card>

      <el-card class="pd-editor">
        <h4>提交代码</h4>
        <el-select v-model="lang" style="margin-bottom: 8px; width: min(220px, 100%)">
          <el-option v-for="l in languages" :key="l.id" :label="l.name" :value="l.id" />
        </el-select>
        <CodeEditor v-model="code" :language="lang" />
        <el-button type="primary" style="margin-top: 12px" :loading="submitting" @click="submit">
          提交
        </el-button>
      </el-card>

      <el-card v-if="lastSub" class="pd-result">
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
    </div>
  </div>
</template>

<style scoped>
/* Same responsive grid as the bank problem page: stacked on phones,
   two columns on desktop, three on ultrawide. */
.pd-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: minmax(0, 1fr);
  align-items: start;
}
@media (min-width: 992px) {
  .pd-grid {
    grid-template-columns: minmax(0, 52fr) minmax(0, 48fr);
  }
  .pd-statement {
    grid-column: 1;
    grid-row: 1 / span 2;
  }
  .pd-editor {
    grid-column: 2;
    grid-row: 1;
  }
  .pd-result {
    grid-column: 2;
    grid-row: 2;
  }
}
@media (min-width: 1920px) {
  .pd-grid {
    gap: 16px;
    grid-template-columns: minmax(0, 5fr) minmax(0, 4fr) minmax(0, 4fr);
  }
  .pd-statement {
    grid-column: 1;
    grid-row: 1;
  }
  .pd-editor {
    grid-column: 2;
    grid-row: 1;
  }
  .pd-result {
    grid-column: 3;
    grid-row: 1;
  }
}
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
</style>
