<script setup lang="ts">
// 题单编辑器（新建/编辑共用）：标题+描述+题目顺序列表（可搜索添加/删除/上下移动）。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Lists, Problems } from '../api/client'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { FormField } from '@/components/ui/form-field'
import { Empty } from '@/components/ui/empty'

const route = useRoute()
const router = useRouter()
const listId = computed(() => (route.params.id ? Number(route.params.id) : 0))
const saving = ref(false)
const title = ref('')
const description = ref('')
const items = ref<{ problem_id: number; title: string; note: string }[]>([])
// 添加题目的搜索
const searchKw = ref('')
const searchResults = ref<{ id: number; title: string }[]>([])

onMounted(async () => {
  if (!listId.value) return
  const r = await Lists.get(listId.value)
  title.value = r.list.title
  description.value = r.list.description
  items.value = r.items.map((it) => ({ problem_id: it.item.problem_id, title: it.title, note: it.item.note }))
})

async function search() {
  if (!searchKw.value.trim()) return
  const r = await Problems.list({ page: 1, size: 20, q: searchKw.value })
  searchResults.value = r.items.map((p) => ({ id: p.id, title: p.title }))
}

function addProblem(p: { id: number; title: string }) {
  if (items.value.some((i) => i.problem_id === p.id)) {
    toast.warning('该题目已在题单中')
    return
  }
  items.value.push({ problem_id: p.id, title: p.title, note: '' })
}

function removeAt(i: number) {
  items.value.splice(i, 1)
}

function move(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= items.value.length) return
  const arr = items.value
  ;[arr[i], arr[j]] = [arr[j], arr[i]]
}

async function save() {
  if (!title.value.trim()) {
    toast.warning('标题不能为空')
    return
  }
  saving.value = true
  try {
    const payload = {
      title: title.value,
      description: description.value,
      problem_ids: items.value.map((i) => i.problem_id),
      notes: Object.fromEntries(items.value.map((i) => [i.problem_id, i.note])),
    }
    if (listId.value) {
      await Lists.update(listId.value, payload)
      toast.success('已保存')
    } else {
      const created = await Lists.create(payload)
      toast.success('题单已创建')
      router.replace(`/lists/${created.id}/edit`)
    }
    router.push(`/lists/${listId.value || ''}`)
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-[1400px]">
    <div class="mb-3 flex items-center gap-3">
      <h3 class="m-0">{{ listId ? `编辑题单 #${listId}` : '新建题单' }}</h3>
      <span class="flex-1" />
      <Button variant="outline" @click="router.back()">返回</Button>
      <Button :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</Button>
    </div>

    <Card class="mb-4">
      <CardContent class="p-6">
        <div class="flex flex-col gap-4">
          <FormField label="题单标题">
            <Input v-model="title" placeholder="例如：2026 秋季新生周练 #1" />
          </FormField>
          <FormField label="描述">
            <Textarea v-model="description" :rows="2" />
          </FormField>
        </div>
      </CardContent>
    </Card>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-12">
      <div class="md:col-span-5">
        <Card>
          <CardContent class="p-6">
            <h4 class="mb-3 mt-0">添加题目</h4>
            <div class="mb-2 flex gap-2">
              <Input v-model="searchKw" placeholder="按标题搜索题库" @keyup.enter="search" />
              <Button variant="secondary" @click="search">搜索</Button>
            </div>
            <div
              v-for="p in searchResults"
              :key="p.id"
              class="flex items-center justify-between border-b border-border px-1 py-1.5"
            >
              <span>#{{ p.id }} {{ p.title }}</span>
              <Button variant="ghost" size="sm" class="text-primary" @click="addProblem(p)">添加</Button>
            </div>
            <Empty v-if="searchResults.length === 0" description="搜索后点添加" class="py-6" />
          </CardContent>
        </Card>
      </div>
      <div class="md:col-span-7">
        <Card>
          <CardContent class="p-6">
            <h4 class="mb-3 mt-0">题目列表（{{ items.length }}）</h4>
            <div
              v-for="(it, i) in items"
              :key="it.problem_id"
              class="flex items-center gap-2 border-b border-border py-1.5"
            >
              <span class="w-7 text-right text-muted-foreground">{{ i + 1 }}</span>
              <span class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap"
                >#{{ it.problem_id }} {{ it.title }}</span
              >
              <Input v-model="it.note" placeholder="备注（如：必做/week1）" class="h-8 w-40 text-xs" />
              <Button variant="ghost" size="sm" :disabled="i === 0" @click="move(i, -1)">↑</Button>
              <Button variant="ghost" size="sm" :disabled="i === items.length - 1" @click="move(i, 1)">↓</Button>
              <Button variant="ghost" size="sm" class="text-destructive" @click="removeAt(i)">移除</Button>
            </div>
            <Empty v-if="items.length === 0" description="从左侧搜索并添加题目" class="py-6" />
          </CardContent>
        </Card>
      </div>
    </div>
  </div>
</template>
