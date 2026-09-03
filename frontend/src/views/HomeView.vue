<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, Submissions } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Stat } from '@/components/ui/stat'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Empty } from '@/components/ui/empty'
import StatusTag from '../components/StatusTag.vue'

// why no top-level await: <script setup> + top-level await makes this an
// async component, and without a <Suspense> boundary the router wedges after
// landing here — every later navigation froze the app (observed in prod).
type SubList = Awaited<ReturnType<typeof Submissions.list>>
const recent = ref<SubList>({ total: 0, items: [] })
const stats = ref<{ by_status: Record<string, number>; ac_problems: number }>({
  by_status: {},
  ac_problems: 0,
})
const loading = ref(true)

onMounted(async () => {
  try {
    recent.value = await Submissions.list({ mine: 1, size: 10 })
    stats.value = (await api.get('/users/me/stats')).data
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="grid gap-4 md:grid-cols-3">
    <Card>
      <CardHeader>
        <CardTitle>我的统计</CardTitle>
      </CardHeader>
      <CardContent>
        <Stat label="已解决题目" :value="stats.ac_problems" />
        <div class="mt-3">
          <p v-for="(n, s) in stats.by_status" :key="s" class="my-1">
            <StatusTag :status="s" /> × {{ n }}
          </p>
        </div>
      </CardContent>
    </Card>
    <Card class="md:col-span-2">
      <CardHeader>
        <CardTitle>我最近的提交</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-20">ID</TableHead>
              <TableHead class="w-20">题目</TableHead>
              <TableHead>状态</TableHead>
              <TableHead class="w-24">耗时</TableHead>
              <TableHead class="w-24">内存</TableHead>
              <TableHead class="w-24">语言</TableHead>
              <TableHead>提交时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in recent.items" :key="row.id">
              <TableCell>{{ row.id }}</TableCell>
              <TableCell>{{ row.problem_id }}</TableCell>
              <TableCell>
                <StatusTag :status="row.status" />
              </TableCell>
              <TableCell>{{ row.time_ms }}</TableCell>
              <TableCell>{{ row.memory_kb }}</TableCell>
              <TableCell>{{ row.language }}</TableCell>
              <TableCell>{{ new Date(row.created_at).toLocaleString() }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <Empty v-if="!loading && recent.items.length === 0" />
      </CardContent>
    </Card>
  </div>
</template>
