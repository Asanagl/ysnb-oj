<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircle, CheckCircle2, Info, TriangleAlert } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const variants = {
  info: { icon: Info, cls: 'border-primary/30 bg-accent-soft text-foreground', iconCls: 'text-primary' },
  success: { icon: CheckCircle2, cls: 'border-ac/30 bg-ac-bg text-ac', iconCls: 'text-ac' },
  warning: { icon: TriangleAlert, cls: 'border-tle/30 bg-tle-bg text-tle', iconCls: 'text-tle' },
  error: { icon: AlertCircle, cls: 'border-wa/30 bg-wa-bg text-wa', iconCls: 'text-wa' },
} as const

type AlertVariant = keyof typeof variants

const props = withDefaults(
  defineProps<{ variant?: AlertVariant; title?: string; class?: string }>(),
  { variant: 'info' },
)

const current = computed(() => variants[props.variant])
</script>

<template>
  <div :class="cn('flex gap-3 rounded-md border px-4 py-3 text-sm', current.cls, props.class)" role="alert">
    <component :is="current.icon" :class="cn('mt-0.5 h-4 w-4 shrink-0', current.iconCls)" />
    <div class="min-w-0">
      <p v-if="title" class="mb-0.5 font-medium">{{ title }}</p>
      <div class="leading-relaxed">
        <slot />
      </div>
    </div>
  </div>
</template>
