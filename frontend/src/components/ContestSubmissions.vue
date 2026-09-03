<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Submissions, errMsg } from '../api/client'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import StatusTag from './StatusTag.vue'

// Contest-scoped submission feed. During a running contest the backend
// already narrows non-managers to their own rows; after the end everyone
// sees the full attempt history. Jury (creator/admin) can cancel/restore
// submissions and trigger single-submission rejudge inline.
const props = defineProps<{ cid: number; isJudge: boolean; frozen: boolean }>()
const items = ref<Awaited<ReturnType<typeof Submissions.list>>['items']>([])
// 封榜期间裁判可切换明文视图（默认遮罩，与全员可见视图一致）
const showTruth = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  const params: Record<string, unknown> = { contest_id: props.cid, size: 50 }
  if (props.isJudge && props.frozen && showTruth.value) {
    params.judge_view = 1
  }
  items.value = (await Submissions.list(params)).items
}

async function toggleCancel(id: number, cancelled: boolean) {
  try {
    await Submissions.cancelContestSubmission(props.cid, id, cancelled)
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function rejudgeOne(id: number) {
  try {
    await Submissions.rejudgeContestSubmission(props.cid, id)
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

onMounted(async () => {
  await load()
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <div class="mb-2 flex flex-wrap items-center gap-3">
      <Button size="sm" variant="outline" @click="load">刷新</Button>
      <label v-if="isJudge && frozen" class="flex items-center gap-2 text-sm">
        <Switch v-model="showTruth" />
        {{ showTruth ? '裁判视图（显示真实判定）' : '遮罩视图' }}
      </label>
      <span class="text-xs text-muted-foreground">
        每 10 秒自动刷新；赛后提交为补题，不影响赛时榜单；被取消的提交计分为零
      </span>
    </div>
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead class="w-16">#</TableHead>
          <TableHead>用户</TableHead>
          <TableHead>题目</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>耗时</TableHead>
          <TableHead>内存</TableHead>
          <TableHead>语言</TableHead>
          <TableHead>补题</TableHead>
          <TableHead>提交时间</TableHead>
          <TableHead v-if="isJudge">裁判</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="row in items" :key="row.id">
          <TableCell class="font-mono">{{ row.id }}</TableCell>
          <TableCell>
            <router-link :to="`/users/${row.user_id}`" class="text-primary hover:underline">
              {{ row.user_id }}
            </router-link>
          </TableCell>
          <TableCell class="font-mono">{{ row.problem_id }}</TableCell>
          <TableCell>
            <StatusTag :status="row.cancelled ? '已取消' : row.status" />
          </TableCell>
          <TableCell class="font-mono">{{ row.time_ms }} ms</TableCell>
          <TableCell class="font-mono">{{ row.memory_kb }} KB</TableCell>
          <TableCell>{{ row.language }}</TableCell>
          <TableCell>{{ row.is_practice ? '是' : '' }}</TableCell>
          <TableCell>{{ new Date(row.created_at).toLocaleString() }}</TableCell>
          <TableCell v-if="isJudge">
            <div class="flex gap-2">
              <Button size="sm" variant="outline" @click="rejudgeOne(row.id)">重判</Button>
              <Button
                size="sm"
                :variant="row.cancelled ? 'secondary' : 'destructive'"
                @click="toggleCancel(row.id, !row.cancelled)"
              >
                {{ row.cancelled ? '恢复' : '取消' }}
              </Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
