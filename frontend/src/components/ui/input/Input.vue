<script setup lang="ts">
import { cn } from '@/lib/utils'

const model = defineModel<string>({ default: '' })

const props = withDefaults(
  defineProps<{
    type?: string
    placeholder?: string
    disabled?: boolean
    class?: string
  }>(),
  { type: 'text' },
)
</script>

<template>
  <input
    v-model="model"
    :type="props.type"
    :placeholder="placeholder"
    :disabled="disabled"
    :class="
      cn(
        'flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  />
</template>
