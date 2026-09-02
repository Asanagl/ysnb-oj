<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Submissions } from '../api/client'
import StatusTag from '../components/StatusTag.vue'

const router = useRouter()
const items = ref<Awaited<ReturnType<typeof Submissions.list>>['items']>([])
const total = ref(0)
const page = ref(1)
const mine = ref(false)
const status = ref('')

async function load() {
  const params: Record<string, unknown> = { page: page.value, size: 20 }
  if (mine.value) params.mine = 1
  if (status.value) params.status = status.value
  const r = await Submissions.list(params)
  items.value = r.items
  total.value = r.total
}

onMounted(load)
</script>

<template>
  <el-card>
    <div style="display: flex; gap: 12px; margin-bottom: 12px">
      <el-checkbox v-model="mine" label="只看我的" @change="page = 1; load()" />
      <el-select v-model="status" placeholder="状态" clearable style="width: 160px" @change="page = 1; load()">
        <el-option v-for="s in ['AC', 'WA', 'TLE', 'MLE', 'RE', 'CE', 'SE', 'PENDING', 'JUDGING']" :key="s" :label="s" :value="s" />
      </el-select>
    </div>
    <el-table :data="items" @row-click="(row: { id: number }) => router.push(`/submissions/${row.id}`)">
      <el-table-column label="#" prop="id" width="80" />
      <el-table-column label="用户" prop="user_id" width="90" />
      <el-table-column label="题目" prop="problem_id" width="90" />
      <el-table-column label="状态">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="耗时" width="100">
        <template #default="{ row }">{{ row.time_ms }} ms</template>
      </el-table-column>
      <el-table-column label="内存" width="110">
        <template #default="{ row }">{{ row.memory_kb }} KB</template>
      </el-table-column>
      <el-table-column label="语言" prop="language" width="110" />
      <el-table-column label="代码量" prop="code_size" width="110" />
      <el-table-column label="提交时间">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
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
