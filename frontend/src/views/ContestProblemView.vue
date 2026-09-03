<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/lib/toast'

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
    toast.error('该题不属于当前比赛')
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
    toast.success(practice.value ? '补题已提交' : '已提交，等待判题')
    pollSubmission(sub.id)
  } catch (e) {
    toast.error(errMsg(e))
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
    <Card class="mb-3">
      <CardContent class="flex flex-wrap items-center gap-3 p-3">
        <Button variant="outline" @click="router.push(`/contests/${cid}`)">返回比赛</Button>
        <strong class="font-semibold">{{ contest.contest.title }}</strong>
        <span class="text-muted-foreground">{{ remaining }}</span>
        <Badge v-if="practice" variant="tle">补题</Badge>
      </CardContent>
    </Card>

    <div
      class="grid grid-cols-1 items-start gap-3 min-[992px]:grid-cols-[minmax(0,52fr)_minmax(0,48fr)] min-[1920px]:grid-cols-[minmax(0,5fr)_minmax(0,4fr)_minmax(0,4fr)] min-[1920px]:gap-4"
    >
      <Card class="min-[992px]:row-span-2 min-[1920px]:row-span-1">
        <CardContent class="p-6">
          <h2 class="text-xl font-semibold">{{ problem.problem.title }}</h2>
          <p class="text-sm text-muted-foreground">
            时限 {{ problem.problem.time_limit_ms }} ms · 内存 {{ problem.problem.mem_limit_mb }} MB
          </p>
          <div class="statement" v-html="statementHTML" />
          <template v-if="samples.length">
            <h4 class="mt-4 font-semibold">样例</h4>
            <div
              v-for="(s, i) in samples"
              :key="i"
              class="mt-2 flex flex-col gap-2 md:flex-row md:gap-3"
            >
              <Textarea :model-value="s.input" readonly :rows="3" placeholder="输入" />
              <Textarea :model-value="s.output" readonly :rows="3" placeholder="输出" />
            </div>
          </template>
        </CardContent>
      </Card>

      <Card>
        <CardContent class="p-6">
          <h4 class="mb-2 font-semibold">提交代码</h4>
          <Select v-model="lang">
            <SelectTrigger class="mb-2 w-[min(220px,100%)]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="l in languages" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
            </SelectContent>
          </Select>
          <CodeEditor v-model="code" :language="lang" />
          <Button class="mt-3" :disabled="submitting" @click="submit">
            {{ submitting ? '提交中…' : '提交' }}
          </Button>
        </CardContent>
      </Card>

      <Card v-if="lastSub">
        <CardContent class="p-6">
          <h4 class="mb-2 flex items-center gap-2 font-semibold">
            最近提交 #{{ lastSub.id }}
            <StatusTag :status="lastSub.status" />
          </h4>
          <p v-if="lastSub.compile_message" class="whitespace-pre-wrap text-muted-foreground">
            {{ lastSub.compile_message }}
          </p>
          <Table v-if="lastSub.cases.length">
            <TableHeader>
              <TableRow>
                <TableHead class="w-[60px]">#</TableHead>
                <TableHead>状态</TableHead>
                <TableHead class="w-[90px]">耗时</TableHead>
                <TableHead class="w-[90px]">内存</TableHead>
                <TableHead>信息</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="c in lastSub.cases" :key="c.index">
                <TableCell>{{ c.index }}</TableCell>
                <TableCell><StatusTag :status="c.status" /></TableCell>
                <TableCell>{{ c.time_ms }}</TableCell>
                <TableCell>{{ c.mem_kb }}</TableCell>
                <TableCell>{{ c.message }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
