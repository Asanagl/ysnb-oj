<script setup lang="ts">
import { cn } from '@/lib/utils'

interface Props {
  label?: string
  error?: string
  hint?: string
  required?: boolean
  class?: string
}

const props = defineProps<Props>()
</script>

<template>
  <div :class="cn('flex flex-col gap-1.5', props.class)">
    <label v-if="label" class="text-sm font-medium">
      {{ label }}<span v-if="required" class="ml-0.5 text-destructive">*</span>
    </label>
    <slot />
    <p v-if="error" class="text-xs text-destructive">{{ error }}</p>
    <p v-else-if="hint" class="text-xs text-muted-foreground">{{ hint }}</p>
  </div>
</template>
