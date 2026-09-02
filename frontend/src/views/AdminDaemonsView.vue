<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Admin, connectWS } from '../api/client'

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
  <el-card>
    <h3>判题机监控</h3>
    <el-alert :title="`等待判题队列：${data.queue_length} 个任务`" type="info" :closable="false" />
    <el-table :data="data.daemons" style="margin-top: 12px">
      <el-table-column label="名称" prop="name" />
      <el-table-column label="状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'online' ? 'success' : 'danger'">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="并发上限" prop="capacity" />
      <el-table-column label="正在判题" prop="active_tasks" />
      <el-table-column label="最近心跳">
        <template #default="{ row }">
          {{ row.last_heartbeat ? new Date(row.last_heartbeat).toLocaleTimeString() : '从未' }}
        </template>
      </el-table-column>
    </el-table>
    <el-alert
      title="在 Linux 判题机上运行 oj-judge --selftest 可验证沙箱环境，然后启动 daemon 即可接入"
      type="info"
      :closable="false"
      style="margin-top: 12px"
    />
  </el-card>
</template>
