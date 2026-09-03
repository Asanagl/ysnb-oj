<script setup lang="ts">
import { DialogTrigger } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{ class?: string }>()
</script>

<template>
  <DialogTrigger :class="cn(props.class)">
    <slot />
  </DialogTrigger>
</template>
