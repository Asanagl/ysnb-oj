<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, Submissions } from '../api/client'
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
  <el-row :gutter="16">
    <el-col :xs="24" :md="8">
      <el-card>
        <h3>我的统计</h3>
        <el-statistic title="已解决题目" :value="stats.ac_problems" />
        <div style="margin-top: 12px">
          <p v-for="(n, s) in stats.by_status" :key="s" style="margin: 4px 0">
            <StatusTag :status="s" /> × {{ n }}
          </p>
        </div>
      </el-card>
    </el-col>
    <el-col :xs="24" :md="16">
      <el-card>
        <h3>我最近的提交</h3>
        <div class="table-scroll">
          <el-table :data="recent.items" size="small">
            <el-table-column label="ID" prop="id" width="80" />
            <el-table-column label="题目" prop="problem_id" width="80" />
            <el-table-column label="状态">
              <template #default="{ row }">
                <StatusTag :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="耗时" prop="time_ms" width="100" />
            <el-table-column label="内存" prop="memory_kb" width="100" />
            <el-table-column label="语言" prop="language" width="100" />
            <el-table-column label="提交时间">
              <template #default="{ row }">
                {{ new Date(row.created_at).toLocaleString() }}
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-card>
    </el-col>
  </el-row>
</template>
