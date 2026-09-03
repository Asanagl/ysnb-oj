<script setup lang="ts">
// 插件系统管理：已注册的题目爬虫 / 刷题平台适配器 / 事件钩子（webhook 目标）。
import { onMounted, ref } from 'vue'
import { Plugins } from '../api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'

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
    toast.warning('请填写名称与 URL')
    return
  }
  try {
    await Plugins.setHook(newName.value.trim(), newURL.value.trim())
    toast.success('已保存')
    newName.value = ''
    newURL.value = ''
    await load()
  } catch (e) {
    toast.error(String((e as Error).message ?? e))
  }
}

async function removeHook(name: string) {
  if (!(await confirmDialog({ title: '删除后该端点不再收到事件，确认？', danger: true }))) return
  await Plugins.deleteHook(name)
  toast.success('已删除')
  await load()
}
</script>

<template>
  <Card v-if="data">
    <CardHeader>
      <CardTitle>插件系统（编译期注册表）</CardTitle>
    </CardHeader>
    <CardContent>
      <dl class="mb-4 divide-y divide-border rounded-md border border-border">
        <div class="grid gap-2 p-3 sm:grid-cols-[220px_1fr] sm:items-center">
          <dt class="text-sm text-muted-foreground">题目爬虫（题库导入）</dt>
          <dd>
            <Badge v-for="p in data.problem_sources" :key="p" class="mr-1.5">{{ p }}</Badge>
            <span v-if="!data.problem_sources.length">无</span>
          </dd>
        </div>
        <div class="grid gap-2 p-3 sm:grid-cols-[220px_1fr] sm:items-center">
          <dt class="text-sm text-muted-foreground">刷题平台适配器（刷题统计报表）</dt>
          <dd>
            <Badge v-for="p in data.submit_fetchers" :key="p" variant="ac" class="mr-1.5">{{ p }}</Badge>
            <span v-if="!data.submit_fetchers.length">无</span>
          </dd>
        </div>
        <div class="grid gap-2 p-3 sm:grid-cols-[220px_1fr] sm:items-center">
          <dt class="text-sm text-muted-foreground">事件钩子</dt>
          <dd>
            <Badge v-for="p in data.event_hooks" :key="p" variant="tle" class="mr-1.5">{{ p }}</Badge>
          </dd>
        </div>
      </dl>

      <h4 class="mb-2 text-sm font-semibold">Webhook 目标（判题完成后 POST JSON：id/status/score/problem_id/contest_id/user_id/at）</h4>
      <div class="mb-3 flex flex-wrap gap-2">
        <Input v-model="newName" placeholder="名称（如 qqcot-bot）" class="w-[200px]" />
        <Input v-model="newURL" placeholder="https://bot.example.com/oj-hook" class="w-[360px]" @keyup.enter="addHook" />
        <Button @click="addHook">添加 / 更新</Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[200px]">名称</TableHead>
            <TableHead>URL</TableHead>
            <TableHead class="w-[100px]">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="[name, url] in Object.entries(data.hook_targets)" :key="name">
            <TableCell>{{ name }}</TableCell>
            <TableCell>{{ url }}</TableCell>
            <TableCell>
              <Button variant="ghost" size="sm" class="text-destructive" @click="removeHook(name)">删除</Button>
            </TableCell>
          </TableRow>
          <TableRow v-if="!Object.keys(data.hook_targets).length">
            <TableCell :colspan="3" class="text-center text-muted-foreground">暂无数据</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
