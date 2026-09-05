<script setup lang="ts">
// 实时判定卡片（Hydro 风格）：判中时测试点圆点条逐个点亮 + n/m 进度；
// 终态显示完整判定名大字 + 耗时/内存/得分 chip + 编译信息。
// 完整判定名是用户要求的口径，仅用于本卡片；全站 StatusTag 短码不动。
import { computed } from 'vue'
import { Loader2 } from 'lucide-vue-next'
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

// 圆点配色（与全站判定色板一致的 CSS 变量）
const DOT: Record<string, string> = {
  AC: 'bg-ac',
  WA: 'bg-wa',
  TLE: 'bg-tle',
  MLE: 'bg-tle',
  RE: 'bg-wa',
  CE: 'bg-pending',
  SE: 'bg-wa',
  SKIPPED: 'bg-muted',
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

const isActive = computed(() =>
  ['PENDING', 'COMPILING', 'JUDGING'].includes(props.submission.status),
)
const fullName = computed(() => FULL_NAME[props.submission.status] ?? props.submission.status)
const tone = computed(() => TONE[props.submission.status] ?? 'text-foreground')

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
  return s ? `#${i} ${s}` : `#${i} 待评测`
}
const progressText = computed(() =>
  isActive.value && total.value > 0 ? ` ${doneCount.value}/${total.value}` : '',
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
      <div class="flex items-center gap-3">
        <span class="text-sm text-muted-foreground">最近提交</span>
        <span class="font-mono text-sm">#{{ submission.id }}</span>
      </div>

      <!-- 状态横条：判中 spinner + n/m；终态完整判定名大字 -->
      <div class="flex items-center gap-3">
        <template v-if="isActive">
          <Loader2 class="size-6 animate-spin text-primary" />
          <span class="text-xl font-bold text-primary">{{ fullName }}</span>
          <span v-if="progressText" class="font-mono text-lg text-muted-foreground">{{ progressText }}</span>
        </template>
        <template v-else>
          <span class="text-3xl font-black tracking-tight" :class="tone">{{ fullName }}</span>
        </template>
      </div>

      <!-- 测试点圆点条（判中逐个点亮，终态全亮） -->
      <div v-if="total > 0" class="flex flex-wrap gap-1.5">
        <span
          v-for="i in total"
          :key="i"
          class="inline-flex h-7 min-w-7 items-center justify-center rounded-md px-1 font-mono text-xs font-semibold transition-colors"
          :class="dotClass(i)"
          :title="dotTitle(i)"
        >
          {{ i }}
        </span>
      </div>

      <!-- 终态指标 chip -->
      <div v-if="showMetrics" class="flex flex-wrap gap-2">
        <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">
          耗时 <span class="font-mono text-foreground">{{ submission.time_ms }}</span> ms
        </span>
        <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">
          内存 <span class="font-mono text-foreground">{{ submission.memory_kb }}</span> KB
        </span>
        <span
          v-if="submission.score !== undefined && submission.score > 0"
          class="rounded-full bg-ac-bg px-3 py-0.5 text-xs text-ac"
        >
          得分 <span class="font-mono font-semibold">{{ submission.score }}</span>
        </span>
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
