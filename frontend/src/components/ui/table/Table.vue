<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <div :class="cn('relative w-full overflow-x-auto', props.class)">
    <table class="w-full caption-bottom text-sm">
      <slot />
    </table>
  </div>
</template>
