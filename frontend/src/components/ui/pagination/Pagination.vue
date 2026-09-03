<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

interface Props {
  page: number
  pageSize: number
  total: number
}

const props = defineProps<Props>()
const emit = defineEmits<{ 'update:page': [page: number] }>()

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))

const pages = computed<(number | 'ellipsis')[]>(() => {
  const last = pageCount.value
  const cur = props.page
  if (last <= 7) return Array.from({ length: last }, (_, i) => i + 1)
  const nums = [...new Set([1, last, cur - 1, cur, cur + 1].filter((n) => n >= 1 && n <= last))].sort(
    (a, b) => a - b,
  )
  const out: (number | 'ellipsis')[] = []
  let prev = 0
  for (const n of nums) {
    if (prev !== 0 && n - prev > 1) out.push('ellipsis')
    out.push(n)
    prev = n
  }
  return out
})

function go(p: number) {
  if (p >= 1 && p <= pageCount.value && p !== props.page) emit('update:page', p)
}
</script>

<template>
  <div :class="cn('flex flex-wrap items-center justify-between gap-2')">
    <span class="text-sm text-muted-foreground">共 {{ total }} 条</span>
    <div class="flex items-center gap-1">
      <Button variant="ghost" size="icon" :disabled="page <= 1" aria-label="上一页" @click="go(page - 1)">
        <ChevronLeft class="h-4 w-4" />
      </Button>
      <template v-for="(p, i) in pages" :key="i">
        <span v-if="p === 'ellipsis'" class="px-1.5 text-sm text-muted-foreground">…</span>
        <Button
          v-else
          size="icon"
          :variant="p === page ? 'default' : 'ghost'"
          @click="go(p)"
        >
          {{ p }}
        </Button>
      </template>
      <Button
        variant="ghost"
        size="icon"
        :disabled="page >= pageCount"
        aria-label="下一页"
        @click="go(page + 1)"
      >
        <ChevronRight class="h-4 w-4" />
      </Button>
    </div>
  </div>
</template>
