<script setup lang="ts">
import { computed } from 'vue'
import { Minus, Plus } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Props {
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  class?: string
}

const props = withDefaults(defineProps<Props>(), { step: 1 })
const model = defineModel<number>({ default: 0 })

function clamp(v: number): number {
  if (props.min !== undefined && v < props.min) return props.min
  if (props.max !== undefined && v > props.max) return props.max
  return v
}

function bump(dir: 1 | -1) {
  model.value = clamp((model.value ?? 0) + dir * props.step)
}

const text = computed<string>({
  get: () => String(model.value ?? ''),
  set: (s) => {
    if (s === '') return
    const n = Number(s)
    if (!Number.isNaN(n)) model.value = clamp(n)
  },
})

const canDec = computed(() => !props.disabled && (props.min === undefined || model.value > props.min))
const canInc = computed(() => !props.disabled && (props.max === undefined || model.value < props.max))

const btnClass =
  'flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md border border-input bg-card text-muted-foreground shadow-sm transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-50'
</script>

<template>
  <div :class="cn('flex items-center gap-1', props.class)">
    <button type="button" :disabled="!canDec" :class="btnClass" @click="bump(-1)">
      <Minus class="h-4 w-4" />
    </button>
    <input
      v-model="text"
      type="number"
      inputmode="numeric"
      :min="min"
      :max="max"
      :step="step"
      :disabled="disabled"
      :class="
        cn(
          'h-9 w-full min-w-0 flex-1 rounded-md border border-input bg-background px-2 py-1 text-center text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
        )
      "
    />
    <button type="button" :disabled="!canInc" :class="btnClass" @click="bump(1)">
      <Plus class="h-4 w-4" />
    </button>
  </div>
</template>
