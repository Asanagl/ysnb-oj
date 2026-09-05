<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { MyProblems, errMsg, type Problem } from '../api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert } from '@/components/ui/alert'
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

// 后台审核队列：pending 题目一览，通过/驳回操作
const items = ref<Problem[]>([])

async function load() {
  items.value = await MyProblems.pending()
}
onMounted(load)

async function review(id: number, action: 'approve' | 'reject') {
  const confirmed = await confirmDialog(
    action === 'approve'
      ? {
          title: '确认通过该题目？',
          description: '通过后题目将进入公开题库（members 可见）',
          confirmText: '通过',
        }
      : {
          title: '确认驳回该题目？',
          description: '驳回后作者可修改后重投',
          confirmText: '驳回',
          danger: true,
        },
  )
  if (!confirmed) return
  try {
    await MyProblems.review(id, action)
    toast.success(action === 'approve' ? '已通过，题目进入公开题库' : '已驳回，作者可修改后重投')
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>题目审核队列</CardTitle>
    </CardHeader>
    <CardContent>
      <Alert variant="info" class="mb-3">
        用户创建的题目在这里审核：通过后进入公开题库（members 可见），驳回后作者可修改重投
      </Alert>
      <Table v-if="items.length > 0">
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">#</TableHead>
            <TableHead>标题</TableHead>
            <TableHead class="w-[90px]">作者 ID</TableHead>
            <TableHead class="w-[110px]">时限</TableHead>
            <TableHead class="w-[180px]">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in items" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>
              <div class="flex flex-wrap items-center gap-1.5">
                <span>{{ row.title }}</span>
                <Badge v-if="(row.source ?? '').includes('外部训练题')" variant="tle" class="text-[11px]">
                  外部导入 · 待补测试数据
                </Badge>
              </div>
            </TableCell>
            <TableCell>{{ row.created_by }}</TableCell>
            <TableCell>{{ row.time_limit_ms }} ms</TableCell>
            <TableCell>
              <div class="flex gap-2">
                <Button size="sm" @click="review(row.id, 'approve')">通过</Button>
                <Button size="sm" variant="destructive" @click="review(row.id, 'reject')">
                  驳回
                </Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Empty v-else description="暂无待审核题目" />
    </CardContent>
  </Card>
</template>
