<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Submissions } from '../api/client'
import { isFinalStatus, useLiveSubmission } from '../composables/useLiveSubmission'
import StatusTag from '../components/StatusTag.vue'
import { Alert } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const route = useRoute()
// 整页绑定实时快照：加载时非终态则订阅 WS（断线降级轮询），终态事件到达后
// refresh 重拉全量（逐 case 明细 / IOI 得分列 / compile_message 随之更新）
const { snap: sub, set: setSub, track: trackSub, refresh: refreshSub } = useLiveSubmission()
// IOI problems: per-case partial scores ride on CaseResult.score
const hasCaseScore = computed(() => sub.value?.cases.some((c) => c.score !== undefined) ?? false)

onMounted(async () => {
  const s = await Submissions.get(route.params.id as string)
  setSub(s)
  if (!isFinalStatus(s.status)) trackSub(s.id, () => void refreshSub())
})
</script>

<template>
  <Card v-if="sub">
    <CardHeader>
      <CardTitle class="flex flex-wrap items-center gap-2 text-lg">
        提交 #{{ sub.id }} · 题目 #{{ sub.problem_id }}
        <StatusTag :status="sub.status" />
        <span v-if="sub.score !== undefined" class="font-mono text-sm text-muted-foreground">
          得分 {{ sub.score }}
        </span>
      </CardTitle>
      <CardDescription>
        语言 {{ sub.language }} · 耗时 {{ sub.time_ms }} ms · 内存 {{ sub.memory_kb }} KB ·
        提交于 {{ new Date(sub.created_at).toLocaleString() }}
      </CardDescription>
    </CardHeader>
    <CardContent class="space-y-4">
      <template v-if="sub.compile_message">
        <h4 class="text-sm font-medium">编译信息</h4>
        <pre class="overflow-auto rounded-md bg-muted p-3 text-left text-sm text-foreground">{{ sub.compile_message }}</pre>
      </template>
      <Table v-if="sub.cases.length">
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">#</TableHead>
            <TableHead>状态</TableHead>
            <TableHead v-if="hasCaseScore" class="w-[90px]">得分</TableHead>
            <TableHead class="w-[110px]">耗时</TableHead>
            <TableHead class="w-[110px]">内存</TableHead>
            <TableHead>信息</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="c in sub.cases" :key="c.index">
            <TableCell>{{ c.index }}</TableCell>
            <TableCell><StatusTag :status="c.status" /></TableCell>
            <TableCell v-if="hasCaseScore" class="font-mono">{{ c.score ?? '' }}</TableCell>
            <TableCell>{{ c.time_ms }} ms</TableCell>
            <TableCell>{{ c.mem_kb }} KB</TableCell>
            <TableCell>{{ c.message }}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <template v-if="sub.code !== undefined">
        <h4 class="text-sm font-medium">代码</h4>
        <pre class="overflow-auto rounded-md bg-muted p-3 text-left text-sm text-foreground">{{ sub.code }}</pre>
      </template>
      <Alert v-else variant="info" title="无权查看该提交代码（比赛结束前仅本人与管理员可见）" />
    </CardContent>
  </Card>
</template>
