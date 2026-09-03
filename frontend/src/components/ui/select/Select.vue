<script setup lang="ts">
import { SelectRoot } from 'reka-ui'

const props = defineProps<{ disabled?: boolean }>()
const model = defineModel<string | number>()
</script>

<template>
  <SelectRoot v-model="model" :disabled="props.disabled">
    <slot />
  </SelectRoot>
</template>
