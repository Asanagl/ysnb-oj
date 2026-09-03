<script setup lang="ts">
import { RadioGroupIndicator, RadioGroupItem } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{
  value: string | number
  disabled?: boolean
  class?: string
}>()
</script>

<template>
  <RadioGroupItem
    :value="value"
    :disabled="disabled"
    :class="
      cn(
        'aspect-square h-4 w-4 cursor-pointer rounded-full border border-input bg-background shadow focus:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary',
        props.class,
      )
    "
  >
    <RadioGroupIndicator class="flex items-center justify-center">
      <span class="h-2 w-2 rounded-full bg-primary" />
    </RadioGroupIndicator>
  </RadioGroupItem>
</template>
