<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Submissions } from '../api/client'
import StatusTag from '../components/StatusTag.vue'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'

const router = useRouter()
const items = ref<Awaited<ReturnType<typeof Submissions.list>>['items']>([])
const total = ref(0)
const page = ref(1)
const mine = ref(false)
const status = ref('')
// reka-ui 的 SelectItem 不允许空字符串 value，用 'ALL' 哨兵表示不筛选状态
const statusSelect = ref('ALL')

const statusOptions = ['AC', 'WA', 'TLE', 'MLE', 'RE', 'CE', 'SE', 'PENDING', 'JUDGING']

async function load() {
  const params: Record<string, unknown> = { page: page.value, size: 20 }
  if (mine.value) params.mine = 1
  if (status.value) params.status = status.value
  const r = await Submissions.list(params)
  items.value = r.items
  total.value = r.total
}

watch(mine, () => {
  page.value = 1
  load()
})

watch(statusSelect, (v) => {
  status.value = v === 'ALL' ? '' : v
  page.value = 1
  load()
})

function onPageChange(p: number) {
  page.value = p
  load()
}

onMounted(load)
</script>

<template>
  <Card>
    <CardContent class="pt-6">
      <div class="mb-3 flex items-center gap-3">
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <Checkbox v-model="mine" />
          只看我的
        </label>
        <Select v-model="statusSelect">
          <SelectTrigger class="w-[160px]">
            <SelectValue placeholder="状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">全部状态</SelectItem>
            <SelectItem v-for="s in statusOptions" :key="s" :value="s">{{ s }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[80px]">#</TableHead>
            <TableHead class="w-[90px]">用户</TableHead>
            <TableHead class="w-[90px]">题目</TableHead>
            <TableHead>状态</TableHead>
            <TableHead class="w-[100px]">耗时</TableHead>
            <TableHead class="w-[110px]">内存</TableHead>
            <TableHead class="w-[110px]">语言</TableHead>
            <TableHead class="w-[110px]">代码量</TableHead>
            <TableHead>提交时间</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="row in items"
            :key="row.id"
            class="cursor-pointer"
            @click="router.push(`/submissions/${row.id}`)"
          >
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.user_id }}</TableCell>
            <TableCell>{{ row.problem_id }}</TableCell>
            <TableCell><StatusTag :status="row.status" /></TableCell>
            <TableCell>{{ row.time_ms }} ms</TableCell>
            <TableCell>{{ row.memory_kb }} KB</TableCell>
            <TableCell>{{ row.language }}</TableCell>
            <TableCell>{{ row.code_size }}</TableCell>
            <TableCell>{{ new Date(row.created_at).toLocaleString() }}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Pagination
        :page="page"
        :page-size="20"
        :total="total"
        class="mt-3"
        @update:page="onPageChange"
      />
    </CardContent>
  </Card>
</template>
