<script setup lang="ts">
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui'
import { Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const checked = defineModel<boolean>({ default: false })

const props = defineProps<{ disabled?: boolean; class?: string }>()
</script>

<template>
  <CheckboxRoot
    v-model="checked"
    :disabled="disabled"
    :class="
      cn(
        'peer h-4 w-4 shrink-0 cursor-pointer rounded border border-input bg-background shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
        props.class,
      )
    "
  >
    <CheckboxIndicator class="flex items-center justify-center text-current">
      <Check class="h-3.5 w-3.5" />
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
