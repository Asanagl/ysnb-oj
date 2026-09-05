<script setup lang="ts">
// 实时判定卡片：提交后无刷新跟踪 排队/编译中 → 评测中 → 终态。
// 终态显示完整判定名（Accepted / Wrong Answer / …）——这是用户单独要求的
// 显示口径，只用于本卡片；全站列表与详情页的 StatusTag 短码保持不变。
import { computed } from 'vue'
import { Loader2 } from 'lucide-vue-next'
import type { Submission } from '../api/client'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

const props = defineProps<{ submission: Submission }>()

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

// 与 StatusTag 同一套语义配色（Badge variant / text-* 均来自主题色板）
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
const judged = computed(
  () =>
    !isActive.value &&
    (props.submission.time_ms > 0 || props.submission.memory_kb > 0),
)
</script>

<template>
  <Card>
    <CardContent class="p-6">
      <h4 class="mb-3 flex items-center gap-2 font-semibold">最近提交 #{{ submission.id }}</h4>

      <!-- 终态：完整判定名大字展示 -->
      <div v-if="!isActive" class="flex flex-wrap items-center gap-3">
        <span class="text-2xl font-bold" :class="tone">{{ fullName }}</span>
        <Badge v-if="submission.score !== undefined && submission.score > 0" variant="ac">
          得分 {{ submission.score }}
        </Badge>
      </div>
      <div v-else class="flex items-center gap-2 text-muted-foreground">
        <Loader2 class="size-5 animate-spin" />
        <span class="text-lg font-semibold">{{ fullName }}</span>
      </div>

      <p v-if="judged" class="mt-2 text-sm text-muted-foreground">
        耗时 {{ submission.time_ms }} ms · 内存 {{ submission.memory_kb }} KB
      </p>

      <p
        v-if="submission.compile_message"
        class="mt-3 whitespace-pre-wrap rounded-md bg-muted p-3 text-sm text-muted-foreground"
      >
        {{ submission.compile_message }}
      </p>

      <RouterLink
        :to="`/submissions/${submission.id}`"
        class="mt-3 inline-block text-sm text-primary hover:underline"
      >
        查看完整评测详情（逐 case / 代码）
      </RouterLink>
    </CardContent>
  </Card>
</template>
