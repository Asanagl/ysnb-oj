<script setup lang="ts">
// API Key 管理：公开 API（/public/*）的机器凭证，生成时唯一一次展示原始值。
import { onMounted, ref } from 'vue'
import { Admin } from '../api/client'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'

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
    toast.warning('请填写 Key 名称')
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
  if (!(await confirmDialog({ title: '吊销后使用该 Key 的调用立即失败，确认？', danger: true }))) return
  await Admin.revokeApiKey(row.id)
  toast.success('已吊销')
  await load()
}

function copyKey() {
  if (minted.value) navigator.clipboard?.writeText(minted.value.key)
  toast.success('已复制')
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>公开 API 密钥（X-API-Key）</CardTitle>
    </CardHeader>
    <CardContent>
      <div class="mb-3 flex gap-2">
        <Input v-model="newName" placeholder="Key 名称（如 qqcot-bot、dashboard）" class="w-80" @keyup.enter="create" />
        <Button :disabled="creating" @click="create">{{ creating ? '生成中…' : '生成新 Key' }}</Button>
      </div>

      <Alert v-if="minted" variant="success" class="mb-3">
        <p class="mb-0.5 font-medium">
          新 Key（仅显示这一次，请立即保存）：
          <code>{{ minted.key }}</code>
          <Button variant="link" size="sm" @click="copyKey">复制</Button>
        </p>
        用法：请求头携带 <code>X-API-Key: &lt;key&gt;</code>（或 ?api_key=）。公开读接口匿名也可用，Key 用于审计与受保护端点。
      </Alert>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">ID</TableHead>
            <TableHead>名称</TableHead>
            <TableHead class="w-[140px]">前缀</TableHead>
            <TableHead class="w-[100px]">状态</TableHead>
            <TableHead class="w-[180px]">创建时间</TableHead>
            <TableHead class="w-[180px]">最近使用</TableHead>
            <TableHead class="w-[100px]">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in keys" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.name }}</TableCell>
            <TableCell>{{ row.prefix }}</TableCell>
            <TableCell>
              <Badge :variant="row.revoked ? 'destructive' : 'ac'">{{ row.revoked ? '已吊销' : '有效' }}</Badge>
            </TableCell>
            <TableCell>{{ new Date(row.created_at).toLocaleString() }}</TableCell>
            <TableCell>{{ row.last_used ? new Date(row.last_used).toLocaleString() : '从未' }}</TableCell>
            <TableCell>
              <Button v-if="!row.revoked" variant="ghost" size="sm" class="text-destructive" @click="revoke(row)">吊销</Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
