<script setup lang="ts">
// API Key 管理：公开 API（/public/*）的机器凭证，生成时唯一一次展示原始值。
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Admin } from '../api/client'

const keys = ref<{ id: number; name: string; prefix: string; revoked: boolean; created_at: string; last_used: string | null }[]>([])
const newName = ref('')
const minted = ref<{ key: string } | null>(null)
const creating = ref(false)

async function load() {
  keys.value = await Admin.apiKeys()
}
onMounted(load)

async function create() {
  if (!newName.value.trim()) {
    ElMessage.warning('请填写 Key 名称')
    return
  }
  creating.value = true
  try {
    minted.value = await Admin.createApiKey(newName.value.trim())
    newName.value = ''
    await load()
  } finally {
    creating.value = false
  }
}

async function revoke(row: { id: number }) {
  await Admin.revokeApiKey(row.id)
  ElMessage.success('已吊销')
  await load()
}

function copyKey() {
  if (minted.value) navigator.clipboard?.writeText(minted.value.key)
  ElMessage.success('已复制')
}
</script>

<template>
  <el-card>
    <template #header>公开 API 密钥（X-API-Key）</template>
    <div style="display: flex; gap: 8px; margin-bottom: 12px">
      <el-input v-model="newName" placeholder="Key 名称（如 qqcot-bot、dashboard）" style="width: 320px" @keyup.enter="create" />
      <el-button type="primary" :loading="creating" @click="create">生成新 Key</el-button>
    </div>

    <el-alert v-if="minted" type="success" :closable="false" style="margin-bottom: 12px">
      <template #title>
        新 Key（仅显示这一次，请立即保存）：
        <code>{{ minted.key }}</code>
        <el-button size="small" text type="primary" @click="copyKey">复制</el-button>
      </template>
      用法：请求头携带 <code>X-API-Key: &lt;key&gt;</code>（或 ?api_key=）。公开读接口匿名也可用，Key 用于审计与受保护端点。
    </el-alert>

    <el-table :data="keys">
      <el-table-column label="ID" prop="id" width="70" />
      <el-table-column label="名称" prop="name" />
      <el-table-column label="前缀" prop="prefix" width="140" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.revoked ? 'danger' : 'success'">{{ row.revoked ? '已吊销' : '有效' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="180">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column label="最近使用" width="180">
        <template #default="{ row }">{{ row.last_used ? new Date(row.last_used).toLocaleString() : '从未' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="100">
        <template #default="{ row }">
          <el-popconfirm v-if="!row.revoked" title="吊销后使用该 Key 的调用立即失败，确认？" @confirm="revoke(row)">
            <template #reference>
              <el-button size="small" type="danger" text>吊销</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>