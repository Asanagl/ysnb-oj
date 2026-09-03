<script setup lang="ts">
import { DialogClose } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <DialogClose :class="cn(props.class)">
    <slot />
  </DialogClose>
</template>
