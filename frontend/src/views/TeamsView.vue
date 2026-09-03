<script setup lang="ts">
// 训练小组列表：全站公开；任何登录用户可建队或凭邀请码加入。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Teams, errMsg, type TeamSummary } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NumberInput } from '@/components/ui/number-input'
import { Badge } from '@/components/ui/badge'
import { Empty } from '@/components/ui/empty'
import { FormField } from '@/components/ui/form-field'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/lib/toast'

const router = useRouter()
const auth = useAuthStore()
const teams = ref<TeamSummary[]>([])
const loading = ref(false)
const createOpen = ref(false)
const joinCode = ref('')
const createForm = ref({ name: '', bio: '', capacity: 3 })

async function load() {
  loading.value = true
  try {
    teams.value = await Teams.all()
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function create() {
  if (!createForm.value.name.trim()) {
    toast.warning('名称不能为空')
    return
  }
  try {
    const t = await Teams.create(createForm.value)
    toast.success(`小组「${t.name}」已创建`)
    createOpen.value = false
    router.push(`/teams/${t.id}`)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function join() {
  if (!joinCode.value.trim()) return
  try {
    const r = await Teams.join(joinCode.value.trim())
    toast.success('已加入小组')
    router.push(`/teams/${r.team_id}`)
  } catch (e) {
    toast.error(errMsg(e))
  }
}
</script>

<template>
  <div>
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h2 class="text-xl font-semibold">训练小组</h2>
      <div class="flex items-center gap-2">
        <Input v-model="joinCode" placeholder="邀请码加入" class="w-40" @keyup.enter="join" />
        <Button @click="join">加入</Button>
        <Button v-if="auth.logged" @click="createOpen = true">创建小组</Button>
      </div>
    </div>

    <p v-if="loading" class="py-10 text-center text-sm text-muted-foreground">加载中…</p>
    <Empty v-else-if="teams.length === 0" description="还没有小组，创建一个吧" />
    <div v-else class="grid gap-4 sm:grid-cols-2 md:grid-cols-3">
      <Card
        v-for="t in teams"
        :key="t.id"
        class="cursor-pointer transition-shadow hover:shadow-md"
        @click="router.push(`/teams/${t.id}`)"
      >
        <CardContent class="p-6">
          <h3 class="mb-1.5 text-base font-semibold">{{ t.name }}</h3>
          <p class="mb-2 min-h-[2em] text-[13px] text-muted-foreground">
            {{ t.bio || '（无简介）' }}
          </p>
          <div class="flex justify-between text-xs text-muted-foreground">
            <span>队长：{{ t.captain }}</span>
            <span>{{ t.member_count }}/{{ t.capacity }} 人</span>
          </div>
          <Badge v-if="t.role === 'captain'" variant="tle" class="mt-1.5">我的小组（队长）</Badge>
          <Badge v-else-if="t.role === 'member'" variant="ac" class="mt-1.5">我的小组</Badge>
        </CardContent>
      </Card>
    </div>

    <Dialog v-model:open="createOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>创建小组</DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <FormField label="小组名称">
            <Input v-model="createForm.name" maxlength="100" />
          </FormField>
          <FormField label="简介">
            <Textarea v-model="createForm.bio" :rows="2" maxlength="300" />
          </FormField>
          <FormField label="人数上限（ICPC 为 3 人）">
            <NumberInput v-model="createForm.capacity" :min="1" :max="5" />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="createOpen = false">取消</Button>
          <Button @click="create">创建</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
