<script setup lang="ts">
// 我的题目：普通用户建题 → pending 审核 → approved 进题库 / rejected 可改后重投
// 新建/编辑统一跳转独立全页编辑器（大视窗），保存后回到本页。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { MyProblems, errMsg } from '../api/client'

const router = useRouter()
const problems = ref<Awaited<ReturnType<typeof MyProblems.list>>>([])

const statusText: Record<string, string> = {
  pending: '待审核',
  approved: '已通过',
  rejected: '已驳回（可修改重投）',
  draft: '草稿',
}
const statusType = (s: string) =>
  s === 'approved' ? 'success' : s === 'rejected' ? 'danger' : 'info'

async function load() {
  problems.value = await MyProblems.list()
}
onMounted(load)

function openCreate() {
  router.push('/my/problems/new')
}

function openEdit(row: { id: number }) {
  router.push(`/my/problems/${row.id}/edit`)
}

// 驳回后一键重投（不进编辑器，直接把状态送回 pending）
async function resubmit(row: { id: number; title: string }) {
  try {
    await MyProblems.update(row.id, { resubmit: true })
    ElMessage.success(`《${row.title}》已重新提交审核`)
    await load()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}
</script>

<template>
  <el-card>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
      <h3 style="margin: 0">我的题目</h3>
      <el-button type="primary" @click="openCreate">新建题目</el-button>
    </div>
    <el-alert
      title="新建题目默认隐藏，管理员审核通过后会出现在公开题库；被驳回可修改后重新提交"
      type="info"
      :closable="false"
      style="margin-bottom: 12px"
    />
    <el-table :data="problems" size="small">
      <el-table-column label="#" prop="id" width="70" />
      <el-table-column label="标题" prop="title" />
      <el-table-column label="状态" width="140">
        <template #default="{ row }">
          <el-tag :type="statusType(row.review_status)">{{ statusText[row.review_status] ?? row.review_status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="170">
        <template #default="{ row }">
          <el-button
            v-if="row.review_status !== 'approved'"
            size="small"
            @click="openEdit(row)"
          >
            编辑
          </el-button>
          <el-popconfirm
            v-if="row.review_status === 'rejected'"
            title="按当前内容重新提交审核？"
            @confirm="resubmit(row)"
          >
            <template #reference>
              <el-button size="small" type="primary" text>重投</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="problems.length === 0" description="还没有出过题，点右上角新建" />
  </el-card>
</template>
