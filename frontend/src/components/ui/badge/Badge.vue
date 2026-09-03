<script lang="ts">
import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva(
  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold transition-colors',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground',
        secondary: 'bg-muted text-muted-foreground',
        outline: 'border border-border text-foreground',
        destructive: 'bg-destructive text-destructive-foreground',
        ac: 'bg-ac-bg text-ac',
        wa: 'bg-wa-bg text-wa',
        tle: 'bg-tle-bg text-tle',
        pending: 'bg-pending-bg text-pending',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

export type BadgeVariants = VariantProps<typeof badgeVariants>
</script>

<script setup lang="ts">
import { cn } from '@/lib/utils'

interface Props {
  variant?: BadgeVariants['variant']
  class?: string
}

const props = withDefaults(defineProps<Props>(), { variant: 'default' })
</script>

<template>
  <span :class="cn(badgeVariants({ variant }), props.class)">
    <slot />
  </span>
</template>
