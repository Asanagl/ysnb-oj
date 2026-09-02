<script setup lang="ts">
// 训练小组列表：全站公开；任何登录用户可建队或凭邀请码加入。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Teams, type TeamSummary } from '../api/client'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const teams = ref<TeamSummary[]>([])
const loading = ref(false)
const createOpen = ref(false)
const joinCode = ref('')
const createForm = ref({ name: '', bio: '', capacity: 3 })

async function load() {
  loading.value = true
  try {
    teams.value = await Teams.all()
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function create() {
  if (!createForm.value.name.trim()) {
    ElMessage.warning('名称不能为空')
    return
  }
  try {
    const t = await Teams.create(createForm.value)
    ElMessage.success(`小组「${t.name}」已创建`)
    createOpen.value = false
    router.push(`/teams/${t.id}`)
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function join() {
  if (!joinCode.value.trim()) return
  try {
    const r = await Teams.join(joinCode.value.trim())
    ElMessage.success('已加入小组')
    router.push(`/teams/${r.team_id}`)
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}
</script>

<template>
  <div v-loading="loading">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; flex-wrap: wrap; gap: 8px">
      <h2 style="margin: 0">训练小组</h2>
      <div style="display: flex; gap: 8px; align-items: center">
        <el-input v-model="joinCode" placeholder="邀请码加入" style="width: 160px" @keyup.enter="join" />
        <el-button @click="join">加入</el-button>
        <el-button v-if="auth.logged" type="primary" @click="createOpen = true">创建小组</el-button>
      </div>
    </div>

    <el-empty v-if="teams.length === 0" description="还没有小组，创建一个吧" />
    <el-row :gutter="16">
      <el-col v-for="t in teams" :key="t.id" :xs="24" :sm="12" :md="8" style="margin-bottom: 16px">
        <el-card shadow="hover" style="cursor: pointer" @click="router.push(`/teams/${t.id}`)">
          <h3 style="margin: 0 0 6px">{{ t.name }}</h3>
          <p style="color: #909399; font-size: 13px; min-height: 2em; margin: 0 0 8px">{{ t.bio || '（无简介）' }}</p>
          <div style="display: flex; justify-content: space-between; color: #909399; font-size: 12px">
            <span>队长：{{ t.captain }}</span>
            <span>{{ t.member_count }}/{{ t.capacity }} 人</span>
          </div>
          <el-tag v-if="t.role === 'captain'" size="small" type="warning" style="margin-top: 6px">我的小组（队长）</el-tag>
          <el-tag v-else-if="t.role === 'member'" size="small" type="success" style="margin-top: 6px">我的小组</el-tag>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog v-model="createOpen" title="创建小组" width="420px">
      <el-form label-position="top">
        <el-form-item label="小组名称">
          <el-input v-model="createForm.name" maxlength="100" />
        </el-form-item>
        <el-form-item label="简介">
          <el-input v-model="createForm.bio" type="textarea" :rows="2" maxlength="300" />
        </el-form-item>
        <el-form-item label="人数上限（ICPC 为 3 人）">
          <el-input-number v-model="createForm.capacity" :min="1" :max="5" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" @click="create">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>
