<script setup lang="ts">
// 题单详情：题目列表 + 我的进度（未做/尝试过/已AC）+ 管理编辑入口。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lists, type ProblemListItemView } from '../api/client'

const router = useRouter()
const listId = computed(() => Number(router.currentRoute.value.params.id))
const list = ref<Awaited<ReturnType<typeof Lists.get>>['list'] | null>(null)
const items = ref<ProblemListItemView[]>([])
const editable = ref(false)
const loading = ref(false)

const progressTag = (p: string) =>
  p === 'ac' ? { text: '已 AC', type: 'success' as const }
  : p === 'tried' ? { text: '尝试过', type: 'warning' as const }
  : { text: '未做', type: 'info' as const }

async function load() {
  loading.value = true
  try {
    const r = await Lists.get(listId.value)
    list.value = r.list
    items.value = r.items
    editable.value = r.editable
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openProblem(row: ProblemListItemView) {
  router.push(`/problems/${row.item.problem_id}`)
}
</script>

<template>
  <div v-loading="loading">
    <template v-if="list">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; flex-wrap: wrap; gap: 8px">
        <div>
          <h2 style="margin: 0">{{ list.title }}</h2>
          <p style="color: #909399; margin: 6px 0 0">{{ list.description }}</p>
        </div>
        <el-button v-if="editable" @click="router.push(`/lists/${list.id}/edit`)">编辑题单</el-button>
      </div>
      <el-table :data="items" @row-click="openProblem" style="cursor: pointer">
        <el-table-column label="#" width="60">
          <template #default="{ $index }">{{ $index + 1 }}</template>
        </el-table-column>
        <el-table-column label="题目" min-width="220">
          <template #default="{ row }">
            <span style="font-weight: 500">{{ row.title }}</span>
          </template>
        </el-table-column>
        <el-table-column label="时限" width="100">
          <template #default="{ row }">{{ row.time_limit_ms }}ms</template>
        </el-table-column>
        <el-table-column label="内存" width="100">
          <template #default="{ row }">{{ row.mem_limit_mb }}MB</template>
        </el-table-column>
        <el-table-column label="备注" prop="item.note" min-width="120" />
        <el-table-column label="我的进度" width="110">
          <template #default="{ row }">
            <el-tag :type="progressTag(row.progress).type" size="small">
              {{ progressTag(row.progress).text }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </div>
</template>
