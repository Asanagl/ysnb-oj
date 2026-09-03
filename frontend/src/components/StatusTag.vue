<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'

const props = defineProps<{ status: string }>()

type Variant = 'ac' | 'wa' | 'tle' | 'pending' | 'default' | 'secondary'

// Verdict → badge variant. SKIPPED: contest test cases skipped after the
// first failure under stop-on-fail (see docs/judge-sandbox.md).
const variantMap: Record<string, Variant> = {
  AC: 'ac',
  WA: 'wa',
  TLE: 'tle',
  MLE: 'tle',
  RE: 'wa',
  CE: 'pending',
  SE: 'pending',
  PENDING: 'pending',
  COMPILING: 'pending',
  JUDGING: 'default',
  SKIPPED: 'secondary',
}

const variant = computed<Variant>(() => variantMap[props.status] ?? 'pending')
</script>

<template>
  <Badge :variant="variant">{{ status }}</Badge>
</template>
