<script setup lang="ts">
// 我的题目：普通用户建题 → pending 审核 → approved 进题库 / rejected 可改后重投
// 新建/编辑统一跳转独立全页编辑器（大视窗），保存后回到本页。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { MyProblems, errMsg } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Empty } from '@/components/ui/empty'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'

const router = useRouter()
const problems = ref<Awaited<ReturnType<typeof MyProblems.list>>>([])

const statusText: Record<string, string> = {
  pending: '待审核',
  approved: '已通过',
  rejected: '已驳回（可修改重投）',
  draft: '草稿',
}
const statusVariant = (s: string) =>
  s === 'approved' ? 'ac' : s === 'rejected' ? 'destructive' : 'secondary'

async function load() {
  problems.value = await MyProblems.list()
}
onMounted(load)

function openCreate() {
  router.push('/my/problems/new')
}

function openEdit(row: { id: number }) {
  router.push(`/my/problems/${row.id}/edit`)
}

// 驳回后一键重投（不进编辑器，直接把状态送回 pending）
async function resubmit(row: { id: number; title: string }) {
  if (!(await confirmDialog({ title: '按当前内容重新提交审核？' }))) return
  try {
    await MyProblems.update(row.id, { resubmit: true })
    toast.success(`《${row.title}》已重新提交审核`)
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
</script>

<template>
  <Card>
    <CardHeader class="flex flex-row items-center justify-between space-y-0">
      <CardTitle>我的题目</CardTitle>
      <Button @click="openCreate">新建题目</Button>
    </CardHeader>
    <CardContent>
      <Alert variant="info" class="mb-3">
        新建题目默认隐藏，管理员审核通过后会出现在公开题库；被驳回可修改后重新提交
      </Alert>
      <Table v-if="problems.length > 0">
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">#</TableHead>
            <TableHead>标题</TableHead>
            <TableHead class="w-[140px]">状态</TableHead>
            <TableHead class="w-[170px]">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in problems" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.title }}</TableCell>
            <TableCell>
              <Badge :variant="statusVariant(row.review_status ?? '')">
                {{ statusText[row.review_status ?? ''] ?? row.review_status }}
              </Badge>
            </TableCell>
            <TableCell>
              <div class="flex gap-2">
                <Button
                  v-if="row.review_status !== 'approved'"
                  size="sm"
                  variant="outline"
                  @click="openEdit(row)"
                >
                  编辑
                </Button>
                <Button
                  v-if="row.review_status === 'rejected'"
                  size="sm"
                  variant="link"
                  @click="resubmit(row)"
                >
                  重投
                </Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Empty v-else description="还没有出过题，点右上角新建" />
    </CardContent>
  </Card>
</template>
