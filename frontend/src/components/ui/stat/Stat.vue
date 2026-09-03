<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{
  label: string
  value: string | number
  sub?: string
  class?: string
}>()
</script>

<template>
  <div :class="cn('flex flex-col gap-1', props.class)">
    <span class="text-xs text-muted-foreground">{{ label }}</span>
    <span class="font-mono text-2xl font-bold">{{ value }}</span>
    <span v-if="sub" class="text-xs text-muted-foreground">{{ sub }}</span>
  </div>
</template>
