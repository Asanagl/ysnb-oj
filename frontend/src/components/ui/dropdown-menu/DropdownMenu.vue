<script setup lang="ts">
import { DropdownMenuRoot } from 'reka-ui'

const open = defineModel<boolean>('open')
</script>

<template>
  <DropdownMenuRoot v-model:open="open">
    <slot />
  </DropdownMenuRoot>
</template>
