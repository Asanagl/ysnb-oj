<script setup lang="ts">
import { Separator } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{ orientation?: 'horizontal' | 'vertical'; class?: string }>(),
  { orientation: 'horizontal' },
)
</script>

<template>
  <Separator
    :orientation="orientation"
    decorative
    :class="
      cn('shrink-0 bg-border', orientation === 'horizontal' ? 'h-px w-full' : 'h-full w-px', props.class)
    "
  />
</template>
