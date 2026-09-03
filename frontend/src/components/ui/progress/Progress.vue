<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{ value: number; class?: string }>()

const clamped = computed(() => Math.min(100, Math.max(0, props.value)))
</script>

<template>
  <div
    :class="cn('h-2 w-full overflow-hidden rounded-full bg-muted', props.class)"
    role="progressbar"
    :aria-valuenow="clamped"
    aria-valuemin="0"
    aria-valuemax="100"
  >
    <div class="h-full bg-primary transition-all" :style="{ width: `${clamped}%` }" />
  </div>
</template>
