<script setup lang="ts">
import { cn } from '@/lib/utils'

const model = defineModel<string>({ default: '' })

const props = defineProps<{
  placeholder?: string
  rows?: number
  disabled?: boolean
  class?: string
}>()
</script>

<template>
  <textarea
    v-model="model"
    :placeholder="placeholder"
    :rows="rows"
    :disabled="disabled"
    :class="
      cn(
        'flex min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  />
</template>
