<script setup lang="ts">
import { SelectTrigger } from 'reka-ui'
import { ChevronDown } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <SelectTrigger
    :class="
      cn(
        'flex h-9 w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus:outline-none focus:ring-2 focus:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[placeholder]:text-muted-foreground',
        props.class,
      )
    "
  >
    <slot />
    <ChevronDown class="h-4 w-4 shrink-0 opacity-50" />
  </SelectTrigger>
</template>
