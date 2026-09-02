<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Contests } from '../api/client'

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
</script>

<template>
  <el-card>
    <h3>比赛列表</h3>
    <el-table :data="contests" @row-click="(row: { id: number }) => router.push(`/contests/${row.id}`)">
      <el-table-column label="#" prop="id" width="80" />
      <el-table-column label="名称" prop="title" />
      <el-table-column label="开始时间">
        <template #default="{ row }">{{ new Date(row.start_time).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column label="结束时间">
        <template #default="{ row }">{{ new Date(row.end_time).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">{{ state(row) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
