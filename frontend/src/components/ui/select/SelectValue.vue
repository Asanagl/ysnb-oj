<script setup lang="ts">
import { SelectValue } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{ placeholder?: string; class?: string }>()
</script>

<template>
  <SelectValue :placeholder="placeholder" :class="cn(props.class)" />
</template>
