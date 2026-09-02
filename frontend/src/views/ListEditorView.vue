<script setup lang="ts">
// 题单编辑器（新建/编辑共用）：标题+描述+题目顺序列表（可搜索添加/删除/上下移动）。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Lists, Problems } from '../api/client'

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
    ElMessage.warning('该题目已在题单中')
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
    ElMessage.warning('标题不能为空')
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
      ElMessage.success('已保存')
    } else {
      const created = await Lists.create(payload)
      ElMessage.success('题单已创建')
      router.replace(`/lists/${created.id}/edit`)
    }
    router.push(`/lists/${listId.value || ''}`)
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="le-root">
    <div style="display: flex; gap: 12px; align-items: center; margin-bottom: 12px">
      <h3 style="margin: 0">{{ listId ? `编辑题单 #${listId}` : '新建题单' }}</h3>
      <span style="flex: 1" />
      <el-button @click="router.back()">返回</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </div>

    <el-card shadow="never" style="margin-bottom: 16px">
      <el-form label-position="top">
        <el-form-item label="题单标题">
          <el-input v-model="title" placeholder="例如：2026 秋季新生周练 #1" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
    </el-card>

    <el-row :gutter="16">
      <el-col :xs="24" :md="10">
        <el-card shadow="never">
          <h4 style="margin-top: 0">添加题目</h4>
          <div style="display: flex; gap: 8px; margin-bottom: 8px">
            <el-input v-model="searchKw" placeholder="按标题搜索题库" @keyup.enter="search" />
            <el-button @click="search">搜索</el-button>
          </div>
          <div v-for="p in searchResults" :key="p.id" class="le-search-row">
            <span>#{{ p.id }} {{ p.title }}</span>
            <el-button size="small" type="primary" text @click="addProblem(p)">添加</el-button>
          </div>
          <el-empty v-if="searchResults.length === 0" description="搜索后点添加" :image-size="60" />
        </el-card>
      </el-col>
      <el-col :xs="24" :md="14">
        <el-card shadow="never">
          <h4 style="margin-top: 0">题目列表（{{ items.length }}）</h4>
          <div v-for="(it, i) in items" :key="it.problem_id" class="le-item-row">
            <span class="le-idx">{{ i + 1 }}</span>
            <span class="le-title">#{{ it.problem_id }} {{ it.title }}</span>
            <el-input v-model="it.note" size="small" placeholder="备注（如：必做/week1）" style="width: 160px" />
            <el-button size="small" text :disabled="i === 0" @click="move(i, -1)">↑</el-button>
            <el-button size="small" text :disabled="i === items.length - 1" @click="move(i, 1)">↓</el-button>
            <el-button size="small" type="danger" text @click="removeAt(i)">移除</el-button>
          </div>
          <el-empty v-if="items.length === 0" description="从左侧搜索并添加题目" :image-size="60" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.le-root {
  max-width: 1400px;
  margin: 0 auto;
}
.le-search-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 4px;
  border-bottom: 1px solid #f0f2f5;
}
.le-item-row {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px solid #f0f2f5;
}
.le-idx {
  width: 28px;
  color: #909399;
  text-align: right;
}
.le-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (max-width: 991.98px) {
  .el-col + .el-col {
    margin-top: 16px;
  }
}
</style>
