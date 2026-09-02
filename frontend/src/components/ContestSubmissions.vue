<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Submissions, errMsg } from '../api/client'
import StatusTag from './StatusTag.vue'

// Contest-scoped submission feed. During a running contest the backend
// already narrows non-managers to their own rows; after the end everyone
// sees the full attempt history. Jury (creator/admin) can cancel/restore
// submissions and trigger single-submission rejudge inline.
const props = defineProps<{ cid: number; isJudge: boolean; frozen: boolean }>()
const items = ref<Awaited<ReturnType<typeof Submissions.list>>['items']>([])
// 封榜期间裁判可切换明文视图（默认遮罩，与全员可见视图一致）
const showTruth = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  const params: Record<string, unknown> = { contest_id: props.cid, size: 50 }
  if (props.isJudge && props.frozen && showTruth.value) {
    params.judge_view = 1
  }
  items.value = (await Submissions.list(params)).items
}

async function toggleCancel(id: number, cancelled: boolean) {
  try {
    await Submissions.cancelContestSubmission(props.cid, id, cancelled)
    await load()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function rejudgeOne(id: number) {
  try {
    await Submissions.rejudgeContestSubmission(props.cid, id)
    await load()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

onMounted(async () => {
  await load()
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <div style="margin-bottom: 8px; display: flex; gap: 12px; align-items: center">
      <el-button size="small" @click="load">刷新</el-button>
      <el-switch
        v-if="isJudge && frozen"
        v-model="showTruth"
        active-text="裁判视图（显示真实判定）"
        inactive-text="遮罩视图"
      />
      <span style="color: #909399; font-size: 12px">
        每 10 秒自动刷新；赛后提交为补题，不影响赛时榜单；被取消的提交计分为零
      </span>
    </div>
    <el-table :data="items" size="small">
      <el-table-column label="#" prop="id" width="70" />
      <el-table-column label="用户" width="90">
        <template #default="{ row }">
          <router-link :to="`/users/${row.user_id}`" style="color: #409eff; text-decoration: none">
            {{ row.user_id }}
          </router-link>
        </template>
      </el-table-column>
      <el-table-column label="题目" prop="problem_id" width="80" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <StatusTag :status="row.cancelled ? '已取消' : row.status" />
        </template>
      </el-table-column>
      <el-table-column label="耗时" width="90">
        <template #default="{ row }">{{ row.time_ms }} ms</template>
      </el-table-column>
      <el-table-column label="内存" width="100">
        <template #default="{ row }">{{ row.memory_kb }} KB</template>
      </el-table-column>
      <el-table-column label="语言" prop="language" width="90" />
      <el-table-column label="补题" width="70">
        <template #default="{ row }">{{ row.is_practice ? '是' : '' }}</template>
      </el-table-column>
      <el-table-column label="提交时间" min-width="150">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column v-if="isJudge" label="裁判" width="190">
        <template #default="{ row }">
          <el-button size="small" @click="rejudgeOne(row.id)">重判</el-button>
          <el-button size="small" :type="row.cancelled ? 'success' : 'danger'" @click="toggleCancel(row.id, !row.cancelled)">
            {{ row.cancelled ? '恢复' : '取消' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
