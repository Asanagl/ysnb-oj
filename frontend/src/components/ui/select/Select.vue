<script setup lang="ts">
import { SelectRoot } from 'reka-ui'

const model = defineModel<string | number>()
</script>

<template>
  <SelectRoot v-model="model">
    <slot />
  </SelectRoot>
</template>
