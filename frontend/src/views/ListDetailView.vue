<script setup lang="ts">
// 题单详情：题目列表 + 我的进度（未做/尝试过/已AC）+ 管理编辑入口。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lists, type ProblemListItemView } from '../api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const router = useRouter()
const listId = computed(() => Number(router.currentRoute.value.params.id))
const list = ref<Awaited<ReturnType<typeof Lists.get>>['list'] | null>(null)
const items = ref<ProblemListItemView[]>([])
const editable = ref(false)
const loading = ref(false)

const progressTag = (p: string) =>
  p === 'ac' ? { text: '已 AC', variant: 'ac' as const }
  : p === 'tried' ? { text: '尝试过', variant: 'tle' as const }
  : { text: '未做', variant: 'secondary' as const }

async function load() {
  loading.value = true
  try {
    const r = await Lists.get(listId.value)
    list.value = r.list
    items.value = r.items
    editable.value = r.editable
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openProblem(row: ProblemListItemView) {
  router.push(`/problems/${row.item.problem_id}`)
}
</script>

<template>
  <div>
    <p v-if="loading && !list" class="text-sm text-muted-foreground">加载中…</p>
    <template v-if="list">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 class="m-0">{{ list.title }}</h2>
          <p class="mt-1.5 mb-0 text-muted-foreground">{{ list.description }}</p>
        </div>
        <Button v-if="editable" @click="router.push(`/lists/${list.id}/edit`)">编辑题单</Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[60px]">#</TableHead>
            <TableHead>题目</TableHead>
            <TableHead class="w-[100px]">时限</TableHead>
            <TableHead class="w-[100px]">内存</TableHead>
            <TableHead>备注</TableHead>
            <TableHead class="w-[110px]">我的进度</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="(row, index) in items"
            :key="row.item.problem_id"
            class="cursor-pointer"
            @click="openProblem(row)"
          >
            <TableCell>{{ index + 1 }}</TableCell>
            <TableCell><span class="font-medium">{{ row.title }}</span></TableCell>
            <TableCell>{{ row.time_limit_ms }}ms</TableCell>
            <TableCell>{{ row.mem_limit_mb }}MB</TableCell>
            <TableCell>{{ row.item.note }}</TableCell>
            <TableCell>
              <Badge :variant="progressTag(row.progress).variant">
                {{ progressTag(row.progress).text }}
              </Badge>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </template>
  </div>
</template>
