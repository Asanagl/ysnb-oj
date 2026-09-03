<script setup lang="ts">
import { Inbox } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = withDefaults(defineProps<{ description?: string; class?: string }>(), {
  description: '暂无数据',
})
</script>

<template>
  <div
    :class="cn('flex flex-col items-center justify-center gap-2 py-10 text-sm text-muted-foreground', props.class)"
  >
    <Inbox class="h-10 w-10 opacity-40" />
    <p>{{ description }}</p>
  </div>
</template>
