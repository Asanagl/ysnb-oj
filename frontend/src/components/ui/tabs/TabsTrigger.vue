<script setup lang="ts">
import { TabsTrigger } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{
  value: string
  disabled?: boolean
  class?: string
}>()
</script>

<template>
  <TabsTrigger
    :value="value"
    :disabled="disabled"
    :class="
      cn(
        'inline-flex cursor-pointer items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-card data-[state=active]:text-foreground data-[state=active]:shadow-sm',
        props.class,
      )
    "
  >
    <slot />
  </TabsTrigger>
</template>
