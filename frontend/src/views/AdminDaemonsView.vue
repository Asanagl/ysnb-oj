<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Admin, connectWS } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const data = ref<Awaited<ReturnType<typeof Admin.daemons>>>({
  daemons: [],
  queue_length: 0,
})
let ws: ReturnType<typeof connectWS> | null = null
let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  data.value = await Admin.daemons()
}

onMounted(async () => {
  await load()
  ws = connectWS(['admin:daemons'], load)
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => {
  ws?.close()
  if (timer) clearInterval(timer)
})
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>判题机监控</CardTitle>
    </CardHeader>
    <CardContent>
      <Alert variant="info">等待判题队列：{{ data.queue_length }} 个任务</Alert>
      <Table class="mt-3">
        <TableHeader>
          <TableRow>
            <TableHead>名称</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>并发上限</TableHead>
            <TableHead>正在判题</TableHead>
            <TableHead>最近心跳</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="daemon in data.daemons" :key="daemon.name">
            <TableCell>{{ daemon.name }}</TableCell>
            <TableCell>
              <Badge :variant="daemon.status === 'online' ? 'ac' : 'destructive'">
                {{ daemon.status }}
              </Badge>
            </TableCell>
            <TableCell>{{ daemon.capacity }}</TableCell>
            <TableCell>{{ daemon.active_tasks }}</TableCell>
            <TableCell>
              {{ daemon.last_heartbeat ? new Date(daemon.last_heartbeat).toLocaleTimeString() : '从未' }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Alert variant="info" class="mt-3">
        在 Linux 判题机上运行 oj-judge --selftest 可验证沙箱环境，然后启动 daemon 即可接入
      </Alert>
    </CardContent>
  </Card>
</template>
