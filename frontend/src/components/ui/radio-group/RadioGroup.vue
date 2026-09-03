<script setup lang="ts">
import { RadioGroupRoot } from 'reka-ui'
import { cn } from '@/lib/utils'

const model = defineModel<string | number>()

const props = defineProps<{ disabled?: boolean; class?: string }>()
</script>

<template>
  <RadioGroupRoot v-model="model" :disabled="disabled" :class="cn('grid gap-2', props.class)">
    <slot />
  </RadioGroupRoot>
</template>
