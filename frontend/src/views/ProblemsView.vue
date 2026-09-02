<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Problems } from '../api/client'

const router = useRouter()
const items = ref<Awaited<ReturnType<typeof Problems.list>>['items']>([])
const total = ref(0)
const page = ref(1)
const q = ref('')

async function load() {
  const r = await Problems.list({ page: page.value, size: 20, q: q.value })
  items.value = r.items
  total.value = r.total
}

onMounted(load)
</script>

<template>
  <el-card>
    <div style="display: flex; gap: 12px; margin-bottom: 12px">
      <el-input v-model="q" placeholder="按标题搜索" style="width: 300px" @change="page = 1; load()" />
    </div>
    <el-table :data="items" @row-click="(row: { id: number }) => router.push(`/problems/${row.id}`)">
      <el-table-column label="#" prop="id" width="90" />
      <el-table-column label="标题" prop="title" />
      <el-table-column label="时限" width="110">
        <template #default="{ row }">{{ row.time_limit_ms }} ms</template>
      </el-table-column>
      <el-table-column label="内存" width="110">
        <template #default="{ row }">{{ row.mem_limit_mb }} MB</template>
      </el-table-column>
      <el-table-column label="类型" width="110">
        <template #default="{ row }">
          {{ row.judge_mode === 'default' ? '标准' : row.judge_mode === 'spj' ? '特判' : '交互' }}
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-model:current-page="page"
      layout="prev, pager, next"
      :total="total"
      :page-size="20"
      style="margin-top: 12px"
      @current-change="load"
    />
  </el-card>
</template>
