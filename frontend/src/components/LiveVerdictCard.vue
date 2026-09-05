<script setup lang="ts">
// 实时判定卡片（Hydro 风格）：判中时测试点圆点条逐个点亮 + n/m 进度；
// 终态显示完整判定名大字 + 耗时/内存/得分统计块 + 编译信息。
// 完整判定名是用户要求的口径，仅用于本卡片；全站 StatusTag 短码不动。
import { computed } from 'vue'
import {
  AlertTriangle, CheckCircle2, Clock, Database, FileWarning,
  Loader2, ServerCrash, SkipForward, XCircle,
} from 'lucide-vue-next'
import type { Submission } from '../api/client'
import type { CaseDot } from '../composables/useLiveSubmission'
import { Card, CardContent } from '@/components/ui/card'

const props = defineProps<{
  submission: Submission
  liveCases?: CaseDot[]
  liveTotal?: number
}>()

const FULL_NAME: Record<string, string> = {
  AC: 'Accepted',
  WA: 'Wrong Answer',
  TLE: 'Time Limit Exceeded',
  MLE: 'Memory Limit Exceeded',
  RE: 'Runtime Error',
  CE: 'Compile Error',
  SE: 'System Error',
  SKIPPED: '已跳过',
  PENDING: '排队中',
  COMPILING: '编译中',
  JUDGING: '评测中',
}

// 圆点配色（判定色底 + 白字，对错一目了然）
const DOT: Record<string, string> = {
  AC: 'bg-ac text-white',
  WA: 'bg-wa text-white',
  TLE: 'bg-tle text-white',
  MLE: 'bg-tle text-white',
  RE: 'bg-wa text-white',
  CE: 'bg-pending text-white',
  SE: 'bg-wa text-white',
  SKIPPED: 'bg-muted text-muted-foreground',
}
// 对错符号：✓ 正确 / ✗ 错误 / – 跳过 / ? 待定
const MARK: Record<string, string> = {
  AC: '✓', WA: '✗', TLE: '✗', MLE: '✗', RE: '✗',
  SE: '✗', SKIPPED: '–', CE: '?',
}
const TONE: Record<string, string> = {
  AC: 'text-ac',
  WA: 'text-wa',
  TLE: 'text-tle',
  MLE: 'text-tle',
  RE: 'text-wa',
  CE: 'text-pending',
  SE: 'text-wa',
  SKIPPED: 'text-muted-foreground',
}
const ICON: Record<string, unknown> = {
  AC: CheckCircle2, WA: XCircle, TLE: Clock, MLE: Database,
  RE: AlertTriangle, CE: FileWarning, SE: ServerCrash, SKIPPED: SkipForward,
}
const ICON_BG: Record<string, string> = {
  AC: 'bg-ac-bg text-ac',
  WA: 'bg-wa-bg text-wa',
  TLE: 'bg-tle-bg text-tle',
  MLE: 'bg-tle-bg text-tle',
  RE: 'bg-wa-bg text-wa',
  CE: 'bg-pending-bg text-pending',
  SE: 'bg-wa-bg text-wa',
  SKIPPED: 'bg-muted text-muted-foreground',
}

const isActive = computed(() =>
  ['PENDING', 'COMPILING', 'JUDGING'].includes(props.submission.status),
)
const fullName = computed(() => FULL_NAME[props.submission.status] ?? props.submission.status)
const tone = computed(() => TONE[props.submission.status] ?? 'text-foreground')
const icon = computed(() => ICON[props.submission.status] ?? CheckCircle2)
const iconBg = computed(() => ICON_BG[props.submission.status] ?? 'bg-muted text-foreground')

// 终态用 snap.cases 全量；判中用 WS 逐 case 推送
const dots = computed<CaseDot[]>(() => {
  if (!isActive.value && props.submission.cases.length) {
    return props.submission.cases.map((c) => ({
      index: c.index, status: c.status, time_ms: c.time_ms,
      memory_kb: c.mem_kb, score: c.score,
    }))
  }
  return props.liveCases ?? []
})
const total = computed(() => {
  if (!isActive.value && props.submission.cases.length) return props.submission.cases.length
  return Math.max(props.liveTotal ?? 0, dots.value.length)
})
const doneCount = computed(() => dots.value.filter((d) => d.status !== 'SKIPPED').length)
const dotStatus = (i: number) => dots.value.find((d) => d.index === i)?.status
const dotClass = (i: number) => {
  const s = dotStatus(i)
  return s ? (DOT[s] ?? 'bg-muted') : 'bg-muted text-muted-foreground/60'
}
const dotTitle = (i: number) => {
  const s = dotStatus(i)
  return s ? `#${i} ${s}${MARK[s] ? ' ' + MARK[s] : ''}` : `#${i} 待评测`
}
// 圆点内容：编号 + 对错符号（✓/✗），无状态时不带符号
const dotLabel = (i: number) => {
  const s = dotStatus(i)
  return s && MARK[s] ? `${i}${MARK[s]}` : `${i}`
}
const progressText = computed(() =>
  isActive.value && total.value > 0 ? `${doneCount.value}/${total.value}` : '',
)
const showMetrics = computed(
  () =>
    !isActive.value &&
    (props.submission.time_ms > 0 || props.submission.memory_kb > 0),
)
</script>

<template>
  <Card>
    <CardContent class="space-y-3 p-5">
      <div class="flex items-center gap-2 text-sm text-muted-foreground">
        最近提交 <span class="font-mono">#{{ submission.id }}</span>
      </div>

      <!-- 状态横条：判中 spinner + n/m；终态 = 色块图标 + 完整判定名 -->
      <div class="flex items-center gap-3">
        <template v-if="isActive">
          <span class="flex size-11 items-center justify-center rounded-xl bg-primary/10">
            <Loader2 class="size-6 animate-spin text-primary" />
          </span>
          <span class="text-2xl font-bold text-primary">{{ fullName }}</span>
          <span v-if="progressText" class="font-mono text-lg text-muted-foreground">{{ progressText }}</span>
        </template>
        <template v-else>
          <span class="flex size-12 items-center justify-center rounded-xl" :class="iconBg">
            <component :is="icon" class="size-7" />
          </span>
          <span class="text-3xl font-black tracking-tight" :class="tone">{{ fullName }}</span>
        </template>
      </div>

      <!-- 测试点圆点条（判中逐个点亮，终态全亮） -->
      <div v-if="total > 0" class="flex flex-wrap gap-1.5">
        <span
          v-for="i in total"
          :key="i"
          class="inline-flex h-8 min-w-8 items-center justify-center rounded-md px-1 font-mono text-xs font-bold transition-colors"
          :class="dotClass(i)"
          :title="dotTitle(i)"
        >
          {{ dotLabel(i) }}
        </span>
      </div>

      <!-- 终态统计块：大号等宽数字，一眼可读 -->
      <div v-if="showMetrics" class="flex items-center gap-5 rounded-lg bg-muted/60 px-4 py-2.5">
        <div>
          <div class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">耗时</div>
          <div class="font-mono text-xl font-bold tabular-nums text-foreground">
            {{ submission.time_ms }}<span class="ml-0.5 text-xs font-normal text-muted-foreground">ms</span>
          </div>
        </div>
        <div class="h-9 w-px bg-border" />
        <div>
          <div class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">内存</div>
          <div class="font-mono text-xl font-bold tabular-nums text-foreground">
            {{ submission.memory_kb }}<span class="ml-0.5 text-xs font-normal text-muted-foreground">KB</span>
          </div>
        </div>
        <template v-if="submission.score !== undefined && submission.score > 0">
          <div class="h-9 w-px bg-border" />
          <div>
            <div class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">得分</div>
            <div class="font-mono text-xl font-bold tabular-nums text-ac">{{ submission.score }}</div>
          </div>
        </template>
      </div>

      <p
        v-if="submission.compile_message"
        class="whitespace-pre-wrap rounded-md bg-muted p-3 font-mono text-xs text-muted-foreground"
      >
        {{ submission.compile_message }}
      </p>

      <RouterLink
        :to="`/submissions/${submission.id}`"
        class="inline-block text-sm text-primary hover:underline"
      >
        查看完整评测详情（逐 case / 代码）
      </RouterLink>
    </CardContent>
  </Card>
</template>
