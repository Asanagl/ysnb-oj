<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <th
    :class="cn('h-10 px-3 text-left align-middle text-xs font-medium text-muted-foreground', props.class)"
  >
    <slot />
  </th>
</template>
