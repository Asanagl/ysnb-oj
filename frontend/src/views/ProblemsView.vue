<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Problems } from '../api/client'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
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
const items = ref<Awaited<ReturnType<typeof Problems.list>>['items']>([])
const total = ref(0)
const page = ref(1)
const q = ref('')

async function load() {
  const r = await Problems.list({ page: page.value, size: 20, q: q.value })
  items.value = r.items
  total.value = r.total
}

function onPageChange(p: number) {
  page.value = p
  load()
}

onMounted(load)
</script>

<template>
  <Card>
    <CardContent class="pt-6">
      <div class="mb-3 flex gap-3">
        <Input
          v-model="q"
          placeholder="按标题搜索"
          class="w-[300px]"
          @change="page = 1; load()"
        />
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[90px]">#</TableHead>
            <TableHead>标题</TableHead>
            <TableHead class="w-[110px]">时限</TableHead>
            <TableHead class="w-[110px]">内存</TableHead>
            <TableHead class="w-[110px]">类型</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="row in items"
            :key="row.id"
            class="cursor-pointer"
            @click="router.push(`/problems/${row.id}`)"
          >
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.title }}</TableCell>
            <TableCell>{{ row.time_limit_ms }} ms</TableCell>
            <TableCell>{{ row.mem_limit_mb }} MB</TableCell>
            <TableCell>
              {{ row.judge_mode === 'default' ? '标准' : row.judge_mode === 'spj' ? '特判' : '交互' }}
            </TableCell>
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
