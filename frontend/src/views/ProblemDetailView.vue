<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  Problems,
  Solutions,
  Submissions,
  errMsg,
  type ProblemSolution,
} from '../api/client'
import { useLiveSubmission } from '../composables/useLiveSubmission'
import { useAuthStore } from '../stores/auth'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import CodeEditor from '../components/CodeEditor.vue'
import LiveVerdictCard from '../components/LiveVerdictCard.vue'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Empty } from '@/components/ui/empty'

const route = useRoute()
const auth = useAuthStore()
const problem = ref<Awaited<ReturnType<typeof Problems.get>> | null>(null)
const solutions = ref<ProblemSolution[]>([])
const languages = ref<{ id: string; name: string }[]>([])
const lang = ref('cpp')
const code = ref('')
const submitting = ref(false)
// 实时判定卡片：WS 推送驱动，断线自动降级轮询（useLiveSubmission）
const { snap: liveSnap, caseDots: liveCases, caseTotal: liveTotal, set: liveSet, track: liveTrack, refresh: liveRefresh } = useLiveSubmission()

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
})

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
    toast.warning('内容不能为空')
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
    toast.success('已保存')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function toggleOfficial(sol: ProblemSolution) {
  try {
    await Solutions.update(route.params.id as string, sol.id, {
      title: sol.title, body_md: sol.body_md, is_official: !sol.is_official,
    })
    solutions.value = await Solutions.list(route.params.id as string)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function deleteSolution(sol: ProblemSolution) {
  try {
    await Solutions.remove(route.params.id as string, sol.id)
    solutions.value = await Solutions.list(route.params.id as string)
  } catch (e) {
    toast.error(errMsg(e))
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
    liveSet(sub)
    liveTrack(sub.id, () => void liveRefresh())
    toast.success('已提交，等待判题')
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    submitting.value = false
  }
}

const topLevel = computed(() => solutions.value.filter((s) => !s.parent_id))
const repliesOf = (id: number) => solutions.value.filter((s) => s.parent_id === id)
const canEditSolution = (sol: ProblemSolution) =>
  auth.user && (sol.user_id === auth.user.id || isModerator.value)
</script>

<template>
  <div v-if="problem">
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-[13fr_11fr]">
      <Card class="p-5">
        <h2 class="text-xl font-semibold">{{ problem.problem.title }}</h2>
        <p class="text-muted-foreground">
          时限 {{ problem.problem.time_limit_ms }} ms · 内存 {{ problem.problem.mem_limit_mb }} MB
          · 测试点 {{ problem.case_count }}
        </p>
        <div class="statement" v-html="statementHTML" />
        <template v-if="samples.length">
          <h4 class="text-base font-semibold">样例</h4>
          <div v-for="(s, i) in samples" :key="i" class="mb-2 grid grid-cols-1 gap-3 md:grid-cols-2">
            <Textarea :model-value="s.input" readonly :rows="3" placeholder="输入" />
            <Textarea :model-value="s.output" readonly :rows="3" placeholder="输出" />
          </div>
        </template>
        <div v-if="problem.problem.hint">
          <h4 class="text-base font-semibold">提示</h4>
          <div class="statement" v-html="renderStatement(problem.problem.hint)" />
        </div>
      </Card>

      <div>
        <Card class="p-5">
          <h4 class="mb-2 text-base font-semibold">提交代码</h4>
          <Select v-model="lang">
            <SelectTrigger class="mb-2 w-[220px]">
              <SelectValue placeholder="选择语言" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="l in languages" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
            </SelectContent>
          </Select>
          <CodeEditor v-model="code" :language="lang" />
          <Button class="mt-3" :disabled="submitting" @click="submit">
            {{ submitting ? '提交中…' : '提交' }}
          </Button>
        </Card>
        <LiveVerdictCard v-if="liveSnap" :submission="liveSnap" :live-cases="liveCases" :live-total="liveTotal" class="mt-3" />
      </div>
    </div>

    <Card class="mt-4 p-5">
      <div class="mb-2.5 flex items-center justify-between">
        <h3 class="m-0 text-lg font-semibold">题解区<template v-if="!solLocked">（{{ solutions.length }}）</template></h3>
        <Button v-if="!solLocked" @click="openCreate()">写题解</Button>
      </div>

      <div v-if="solLocked" class="flex flex-col items-center gap-2 py-10 text-center">
        <span class="text-[42px]">🔒</span>
        <p class="m-0 text-sm text-muted-foreground">{{ solLocked }}</p>
        <p class="m-0 text-[13px] text-muted-foreground">
          提交一次本题（任意结果）即可解锁题解区；比赛题目将在比赛结束后开放。
        </p>
      </div>

      <template v-else>
      <div
        v-for="sol in topLevel"
        :key="sol.id"
        class="mb-3 rounded-lg border px-4 py-3"
        :class="sol.is_official ? 'border-ac bg-ac-bg/40' : 'border-border'"
      >
        <div class="mb-2 flex items-center gap-2.5">
          <Badge v-if="sol.is_official" variant="ac">官方题解</Badge>
          <router-link :to="`/users/${sol.user_id}`" class="font-semibold text-foreground no-underline hover:text-primary">{{ sol.username }}</router-link>
          <span class="text-xs text-muted-foreground">{{ new Date(sol.created_at).toLocaleString() }}</span>
          <span class="flex-1" />
          <Button v-if="canEditSolution(sol)" variant="ghost" size="sm" @click="openEdit(sol)">编辑</Button>
          <Button v-if="isModerator" variant="ghost" size="sm" @click="toggleOfficial(sol)">
            {{ sol.is_official ? '取消置顶' : '设为官方' }}
          </Button>
          <Button v-if="canEditSolution(sol)" variant="ghost" size="sm" class="text-destructive" @click="deleteSolution(sol)">删除</Button>
        </div>
        <div v-if="sol.title" class="mb-1.5 font-semibold">{{ sol.title }}</div>
        <div class="sol-body wysiwyg-render" v-html="sol.body_md" />
        <div class="mt-1.5">
          <Button variant="ghost" size="sm" @click="openCreate(sol)">回复</Button>
        </div>
        <div v-for="r in repliesOf(sol.id)" :key="r.id" class="ml-6 mt-2.5 border-l-[3px] border-border px-0 py-2 pl-3.5">
          <div class="mb-2 flex items-center gap-2.5">
            <router-link :to="`/users/${r.user_id}`" class="font-semibold text-foreground no-underline hover:text-primary">{{ r.username }}</router-link>
            <span class="text-xs text-muted-foreground">{{ new Date(r.created_at).toLocaleString() }}</span>
            <span class="flex-1" />
            <Button v-if="canEditSolution(r)" variant="ghost" size="sm" @click="openEdit(r)">编辑</Button>
            <Button v-if="canEditSolution(r)" variant="ghost" size="sm" class="text-destructive" @click="deleteSolution(r)">删除</Button>
          </div>
          <div class="sol-body wysiwyg-render" v-html="r.body_md" />
        </div>
      </div>
      <Empty v-if="topLevel.length === 0" description="还没有题解，来写第一篇吧" />

      </template>

      <Dialog v-model:open="editorOpen">
        <DialogContent class="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{{ editorMode === 'edit' ? '编辑题解' : replyTo ? `回复 @${replyTo.username}` : '写题解' }}</DialogTitle>
          </DialogHeader>
          <Input v-if="!replyTo" v-model="solForm.title" placeholder="标题（可选）" />
          <MarkdownEditor v-model="solForm.body_html" placeholder="书写题解内容，支持加粗/标题/代码块/图片/链接…" />
          <DialogFooter>
            <Button variant="outline" @click="editorOpen = false">取消</Button>
            <Button @click="saveSolution">保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  </div>
</template>

<style scoped>
.sol-body {
  line-height: 1.75;
}
.wysiwyg-render :deep(p) {
  margin: 6px 0;
}
.wysiwyg-render :deep(pre) {
  background: var(--muted);
  color: var(--foreground);
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}
.wysiwyg-render :deep(img) {
  max-width: 100%;
}
.wysiwyg-render :deep(blockquote) {
  border-left: 3px solid var(--primary);
  padding-left: 12px;
  color: var(--muted-foreground);
  margin-left: 0;
}
</style>
