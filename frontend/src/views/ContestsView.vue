<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Contests } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Empty } from '@/components/ui/empty'

const router = useRouter()
const contests = ref<Awaited<ReturnType<typeof Contests.list>>>([])

onMounted(async () => {
  contests.value = await Contests.list()
})

function state(c: { start_time: string; end_time: string }) {
  const now = Date.now()
  if (now < new Date(c.start_time).getTime()) return '未开始'
  if (now > new Date(c.end_time).getTime()) return '已结束'
  return '进行中'
}

function stateVariant(s: string): 'ac' | 'pending' | 'secondary' {
  if (s === '进行中') return 'ac'
  if (s === '未开始') return 'pending'
  return 'secondary'
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>比赛列表</CardTitle>
    </CardHeader>
    <CardContent>
      <Empty v-if="contests.length === 0" />
      <Table v-else>
        <TableHeader>
          <TableRow>
            <TableHead class="w-20">#</TableHead>
            <TableHead>名称</TableHead>
            <TableHead>开始时间</TableHead>
            <TableHead>结束时间</TableHead>
            <TableHead class="w-28">状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="c in contests"
            :key="c.id"
            class="cursor-pointer"
            @click="router.push(`/contests/${c.id}`)"
          >
            <TableCell>{{ c.id }}</TableCell>
            <TableCell>{{ c.title }}</TableCell>
            <TableCell>{{ new Date(c.start_time).toLocaleString() }}</TableCell>
            <TableCell>{{ new Date(c.end_time).toLocaleString() }}</TableCell>
            <TableCell>
              <Badge :variant="stateVariant(state(c))">{{ state(c) }}</Badge>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
