<script setup lang="ts">
// 插件系统管理：已注册的题目爬虫 / 刷题平台适配器 / 事件钩子（webhook 目标）。
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plugins } from '../api/client'

const data = ref<{
  problem_sources: string[]
  submit_fetchers: string[]
  event_hooks: string[]
  hook_targets: Record<string, string>
} | null>(null)

const newName = ref('')
const newURL = ref('')

async function load() {
  data.value = await Plugins.list()
}
onMounted(load)

async function addHook() {
  if (!newName.value.trim() || !newURL.value.trim()) {
    ElMessage.warning('请填写名称与 URL')
    return
  }
  try {
    await Plugins.setHook(newName.value.trim(), newURL.value.trim())
    ElMessage.success('已保存')
    newName.value = ''
    newURL.value = ''
    await load()
  } catch (e) {
    ElMessage.error(String((e as Error).message ?? e))
  }
}

async function removeHook(name: string) {
  await Plugins.deleteHook(name)
  ElMessage.success('已删除')
  await load()
}
</script>

<template>
  <el-card v-if="data">
    <template #header>插件系统（编译期注册表）</template>
    <el-descriptions :column="1" border style="margin-bottom: 16px">
      <el-descriptions-item label="题目爬虫（题库导入）">
        <el-tag v-for="p in data.problem_sources" :key="p" style="margin-right: 6px">{{ p }}</el-tag>
        <span v-if="!data.problem_sources.length">无</span>
      </el-descriptions-item>
      <el-descriptions-item label="刷题平台适配器（刷题统计报表）">
        <el-tag v-for="p in data.submit_fetchers" :key="p" type="success" style="margin-right: 6px">{{ p }}</el-tag>
        <span v-if="!data.submit_fetchers.length">无</span>
      </el-descriptions-item>
      <el-descriptions-item label="事件钩子">
        <el-tag v-for="p in data.event_hooks" :key="p" type="warning" style="margin-right: 6px">{{ p }}</el-tag>
      </el-descriptions-item>
    </el-descriptions>

    <h4>Webhook 目标（判题完成后 POST JSON：id/status/score/problem_id/contest_id/user_id/at）</h4>
    <div style="display: flex; gap: 8px; margin-bottom: 12px">
      <el-input v-model="newName" placeholder="名称（如 qqcot-bot）" style="width: 200px" />
      <el-input v-model="newURL" placeholder="https://bot.example.com/oj-hook" style="width: 360px" @keyup.enter="addHook" />
      <el-button type="primary" @click="addHook">添加 / 更新</el-button>
    </div>
    <el-table :data="Object.entries(data.hook_targets).map(([name, url]) => ({ name, url }))">
      <el-table-column label="名称" prop="name" width="200" />
      <el-table-column label="URL" prop="url" />
      <el-table-column label="操作" width="100">
        <template #default="{ row }">
          <el-popconfirm title="删除后该端点不再收到事件，确认？" @confirm="removeHook(row.name)">
            <template #reference>
              <el-button size="small" type="danger" text>删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>