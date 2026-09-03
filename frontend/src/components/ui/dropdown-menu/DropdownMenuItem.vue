<script setup lang="ts">
import { DropdownMenuItem } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{
  danger?: boolean
  disabled?: boolean
  class?: string
}>()
</script>

<template>
  <DropdownMenuItem
    :disabled="disabled"
    :class="
      cn(
        'relative flex cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors data-[disabled]:pointer-events-none data-[highlighted]:bg-muted data-[disabled]:opacity-50',
        danger && 'text-destructive data-[highlighted]:bg-wa-bg',
        props.class,
      )
    "
  >
    <slot />
  </DropdownMenuItem>
</template>
