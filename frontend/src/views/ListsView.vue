<script setup lang="ts">
// 题单列表页：全站公开，所有人可浏览；setter/admin/创建者可新建。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lists, type ProblemListSummary } from '../api/client'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const lists = ref<ProblemListSummary[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    lists.value = await Lists.all()
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div v-loading="loading">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
      <h2 style="margin: 0">训练题单</h2>
      <el-button
        v-if="auth.canManage"
        type="primary"
        @click="router.push('/lists/new')"
      >
        新建题单
      </el-button>
    </div>
    <el-empty v-if="lists.length === 0" description="还没有题单" />
    <el-row :gutter="16">
      <el-col v-for="l in lists" :key="l.id" :xs="24" :sm="12" :md="8" style="margin-bottom: 16px">
        <el-card shadow="hover" style="cursor: pointer" @click="router.push(`/lists/${l.id}`)">
          <h3 style="margin: 0 0 8px">{{ l.title }}</h3>
          <p style="color: #909399; font-size: 13px; min-height: 2em; margin: 0 0 8px">{{ l.description || '（无描述）' }}</p>
          <div style="display: flex; justify-content: space-between; color: #909399; font-size: 12px">
            <span>{{ l.problem_count }} 题</span>
            <span>{{ new Date(l.created_at).toLocaleDateString() }}</span>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>
