<script setup lang="ts">
import { DropdownMenuContent, DropdownMenuPortal } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string; align?: 'start' | 'center' | 'end' }>()
</script>

<template>
  <DropdownMenuPortal>
    <DropdownMenuContent
      :side-offset="4"
      :align="align ?? 'center'"
      :class="
        cn(
          'z-50 min-w-32 overflow-hidden rounded-md border border-border bg-card p-1 text-card-foreground shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
          props.class,
        )
      "
    >
      <slot />
    </DropdownMenuContent>
  </DropdownMenuPortal>
</template>
