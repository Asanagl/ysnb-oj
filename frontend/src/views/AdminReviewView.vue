<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { MyProblems, errMsg, type Problem } from '../api/client'

// 后台审核队列：pending 题目一览，通过/驳回操作
const items = ref<Problem[]>([])

async function load() {
  items.value = await MyProblems.pending()
}
onMounted(load)

async function review(id: number, action: 'approve' | 'reject') {
  try {
    await MyProblems.review(id, action)
    ElMessage.success(action === 'approve' ? '已通过，题目进入公开题库' : '已驳回，作者可修改后重投')
    await load()
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}
</script>

<template>
  <el-card>
    <h3 style="margin-top: 0">题目审核队列</h3>
    <el-alert
      title="用户创建的题目在这里审核：通过后进入公开题库（members 可见），驳回后作者可修改重投"
      type="info"
      :closable="false"
      style="margin-bottom: 12px"
    />
    <el-table :data="items" size="small">
      <el-table-column label="#" prop="id" width="70" />
      <el-table-column label="标题" prop="title" />
      <el-table-column label="作者 ID" prop="created_by" width="90" />
      <el-table-column label="时限" width="110">
        <template #default="{ row }">{{ row.time_limit_ms }} ms</template>
      </el-table-column>
      <el-table-column label="操作" width="180">
        <template #default="{ row }">
          <el-button size="small" type="success" @click="review(row.id, 'approve')">通过</el-button>
          <el-button size="small" type="danger" @click="review(row.id, 'reject')">驳回</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="items.length === 0" description="暂无待审核题目" />
  </el-card>
</template>
