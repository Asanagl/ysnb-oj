<script setup lang="ts">
import { DropdownMenuTrigger } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string; asChild?: boolean }>()
</script>

<template>
  <DropdownMenuTrigger :as-child="asChild" :class="cn('outline-none', props.class)">
    <slot />
  </DropdownMenuTrigger>
</template>
