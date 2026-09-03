<script setup lang="ts">
// 题单列表页：全站公开，所有人可浏览；setter/admin/创建者可新建。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lists, type ProblemListSummary } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Empty } from '@/components/ui/empty'

const router = useRouter()
const auth = useAuthStore()
const lists = ref<ProblemListSummary[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    lists.value = await Lists.all()
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div>
    <div class="mb-3 flex items-center justify-between">
      <h2 class="m-0 text-xl font-semibold">训练题单</h2>
      <Button v-if="auth.canManage" @click="router.push('/lists/new')">
        新建题单
      </Button>
    </div>
    <Empty v-if="!loading && lists.length === 0" description="还没有题单" />
    <div class="grid gap-4 sm:grid-cols-2 md:grid-cols-3">
      <Card
        v-for="l in lists"
        :key="l.id"
        class="cursor-pointer transition-shadow hover:shadow-md"
        @click="router.push(`/lists/${l.id}`)"
      >
        <CardContent class="p-6">
          <h3 class="mb-2 mt-0">{{ l.title }}</h3>
          <p class="mb-2 mt-0 min-h-[2em] text-[13px] text-muted-foreground">
            {{ l.description || '（无描述）' }}
          </p>
          <div class="flex justify-between text-xs text-muted-foreground">
            <span>{{ l.problem_count }} 题</span>
            <span>{{ new Date(l.created_at).toLocaleDateString() }}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
