<script setup lang="ts">
// 日志查看器：读取 oj-api / oj-judge 的 journald 最近日志（admin 网页端）。
// 后端以固定 argv 调 journalctl，此处仅做 unit 选择、关键字过滤与行数控制。
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Admin } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export interface LogEntry {
  ts: string
  unit: string
  level: string
  msg: string
}

const entries = ref<LogEntry[]>([])
const unit = ref<'all' | 'api' | 'judge'>('all')
const q = ref('')
const lines = ref(200)
const loading = ref(false)
const error = ref('')
let timer: ReturnType<typeof setInterval> | null = null

const levelVariant: Record<string, 'ac' | 'wa' | 'tle' | 'secondary' | 'destructive'> = {
  error: 'destructive',
  warn: 'tle',
  info: 'ac',
  debug: 'secondary',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = await Admin.logs(unit.value, q.value, lines.value)
    entries.value = r.entries
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  timer = setInterval(load, 15000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <Card>
    <CardHeader class="flex flex-row items-center justify-between space-y-0">
      <CardTitle>日志查看器（oj-api / oj-judge）</CardTitle>
      <Button variant="outline" size="sm" :disabled="loading" @click="load">刷新</Button>
    </CardHeader>
    <CardContent>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Select v-model="unit" class="w-40" @update:model-value="load">
          <SelectTrigger><SelectValue placeholder="进程" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部进程</SelectItem>
            <SelectItem value="api">API 服务</SelectItem>
            <SelectItem value="judge">判题机</SelectItem>
          </SelectContent>
        </Select>
        <Input
          v-model="q"
          placeholder="关键字过滤（如 SE、submission、error）"
          class="max-w-xs"
          @keyup.enter="load"
        />
        <Select v-model="lines" class="w-28" @update:model-value="load">
          <SelectTrigger><SelectValue placeholder="行数" /></SelectTrigger>
          <SelectContent>
            <SelectItem :value="100">100 行</SelectItem>
            <SelectItem :value="200">200 行</SelectItem>
            <SelectItem :value="500">500 行</SelectItem>
            <SelectItem :value="1000">1000 行</SelectItem>
          </SelectContent>
        </Select>
        <Button size="sm" :disabled="loading" @click="load">查询</Button>
        <span class="text-xs text-muted-foreground">每 15 秒自动刷新</span>
      </div>

      <div v-if="error" class="mb-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
        {{ error }}
      </div>

      <div class="max-h-[65vh] overflow-auto rounded-md border border-border bg-card font-mono text-xs">
        <div
          v-for="(e, i) in entries"
          :key="i"
          class="flex gap-3 border-b border-border/60 px-3 py-1.5 last:border-0 hover:bg-muted/50"
        >
          <span class="shrink-0 text-muted-foreground">{{ e.ts }}</span>
          <Badge :variant="levelVariant[e.level] ?? 'secondary'" class="shrink-0 font-sans">
            {{ e.level }}
          </Badge>
          <span class="whitespace-pre-wrap break-all">{{ e.msg }}</span>
        </div>
        <div v-if="entries.length === 0 && !loading" class="px-3 py-8 text-center text-muted-foreground">
          暂无日志
        </div>
      </div>
    </CardContent>
  </Card>
</template>
