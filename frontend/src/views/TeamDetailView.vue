<script setup lang="ts">
// 小组详情：成员列表、队内公告、共享题单、队内排行；队长管理面板。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Teams, type TeamDetail, type TeamBoardRow } from '../api/client'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Empty } from '@/components/ui/empty'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'

const router = useRouter()
const teamId = computed(() => Number(router.currentRoute.value.params.id))
const detail = ref<TeamDetail | null>(null)
const board = ref<TeamBoardRow[]>([])
const loading = ref(false)
const noticeContent = ref('')
// 队长管理：按用户名拉人 / 分享题单
const pullName = ref('')
const myLists = ref<{ id: number; title: string }[]>([])
const shareListId = ref<number | undefined>()

const isCaptain = computed(() => detail.value?.team.role === 'captain')

async function load() {
  loading.value = true
  try {
    detail.value = await Teams.get(teamId.value)
    board.value = await Teams.board(teamId.value)
    if (isCaptain.value) {
      try {
        const r = await fetch('/api/v1/lists', { headers: { Authorization: `Bearer ${localStorage.getItem('oj_token')}` } })
        myLists.value = (await r.json()) as { id: number; title: string }[]
      } catch { /* non-fatal */ }
    }
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function leave() {
  if (!(await confirmDialog({ title: '确定退出小组？', danger: true }))) return
  try {
    const r = await Teams.leave(teamId.value)
    if ((r as { deleted?: boolean }).deleted) {
      toast.success('小组已解散')
      router.push('/teams')
    } else {
      toast.success('已退出')
      router.push('/teams')
    }
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function pull() {
  if (!pullName.value.trim()) return
  try {
    // resolve username -> id via the global user search (login-gated)
    const r = await fetch(`/api/v1/users/search?q=${encodeURIComponent(pullName.value)}`, {
      headers: { Authorization: `Bearer ${localStorage.getItem('oj_token')}` },
    })
    const users = (await r.json()) as { id: number; username: string }[]
    const u = users.find((x) => x.username === pullName.value.trim())
    if (!u) {
      toast.error('找不到该用户名')
      return
    }
    await Teams.pull(teamId.value, u.id)
    toast.success('已拉入')
    pullName.value = ''
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function kick(uid: number, name: string) {
  if (!(await confirmDialog({ title: `移除 ${name}？`, danger: true }))) return
  try {
    await Teams.kick(teamId.value, uid)
    toast.success('已移除')
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function transfer(uid: number) {
  try {
    await Teams.transfer(teamId.value, uid)
    toast.success('队长已转让')
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function regenInvite() {
  try {
    await Teams.regenerateInvite(teamId.value)
    toast.success('邀请码已重置')
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function postNotice() {
  if (!noticeContent.value.trim()) return
  try {
    await Teams.createNotice(teamId.value, noticeContent.value)
    noticeContent.value = ''
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function delNotice(nid: number) {
  try {
    await Teams.deleteNotice(teamId.value, nid)
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function share() {
  if (!shareListId.value) return
  try {
    await Teams.shareList(teamId.value, shareListId.value)
    toast.success('题单已共享')
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function unshare(listId: number) {
  try {
    await Teams.unshareList(teamId.value, listId)
    await load()
  } catch (e) {
    toast.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}
</script>

<template>
  <div :aria-busy="loading">
    <template v-if="detail">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 class="m-0 text-xl font-semibold">{{ detail.team.name }}</h2>
          <p class="mt-1.5 text-sm text-muted-foreground">{{ detail.team.bio }}</p>
        </div>
        <div class="flex items-center gap-2">
          <Badge v-if="detail.team.role === 'captain'" variant="tle">队长</Badge>
          <Badge v-else-if="detail.team.role === 'member'" variant="ac">队员</Badge>
          <Button v-if="detail.team.role !== 'none'" variant="ghost" class="text-destructive" @click="leave">退出小组</Button>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-12">
        <!-- 成员 + 队长管理 -->
        <div class="md:col-span-5">
          <Card class="mb-4">
            <CardContent class="p-6">
              <h4 class="mb-3 text-base font-semibold">成员（{{ detail.team.member_count }}/{{ detail.team.capacity }}）</h4>
              <div
                v-for="m in detail.team.members"
                :key="m.user_id"
                class="flex items-center gap-1.5 border-b border-border py-1.5"
              >
                <router-link :to="`/users/${m.user_id}`" class="flex-1">
                  {{ m.nickname || m.username }}
                  <Badge v-if="m.is_captain" variant="tle" class="ml-1">队长</Badge>
                </router-link>
                <template v-if="isCaptain && !m.is_captain">
                  <Button size="sm" variant="ghost" @click="transfer(m.user_id)">转让队长</Button>
                  <Button size="sm" variant="ghost" class="text-destructive" @click="kick(m.user_id, m.nickname || m.username)">移除</Button>
                </template>
              </div>
              <template v-if="isCaptain">
                <Separator class="my-4" />
                <div class="mb-2 flex gap-2">
                  <Input v-model="pullName" class="h-8 flex-1" placeholder="按用户名拉人" @keyup.enter="pull" />
                  <Button size="sm" @click="pull">拉人</Button>
                </div>
                <div class="flex items-center gap-2">
                  <code class="flex-1 rounded bg-muted px-2 py-1 text-sm">{{ detail.team.invite_code }}</code>
                  <Button size="sm" @click="regenInvite">重置邀请码</Button>
                </div>
              </template>
            </CardContent>
          </Card>

          <Card class="mb-4">
            <CardContent class="p-6">
              <h4 class="mb-3 text-base font-semibold">共享题单</h4>
              <div
                v-for="l in detail.lists"
                :key="l.id"
                class="flex items-center gap-1.5 border-b border-border py-1.5"
              >
                <router-link :to="`/lists/${l.id}`" class="flex-1">{{ l.title }}（{{ l.items }} 题）</router-link>
                <Button v-if="isCaptain" size="sm" variant="ghost" class="text-destructive" @click="unshare(l.id)">取消共享</Button>
              </div>
              <Empty v-if="detail.lists.length === 0" description="还没有共享题单" class="py-4" />
              <template v-if="isCaptain">
                <div class="mt-2 flex gap-2">
                  <Select v-model="shareListId">
                    <SelectTrigger class="h-8 flex-1">
                      <SelectValue placeholder="选择我的题单" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem v-for="l in myLists" :key="l.id" :value="l.id">{{ l.title }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button size="sm" @click="share">共享</Button>
                </div>
              </template>
            </CardContent>
          </Card>
        </div>

        <!-- 公告 + 排行 -->
        <div class="md:col-span-7">
          <Card class="mb-4">
            <CardContent class="p-6">
              <h4 class="mb-3 text-base font-semibold">队内公告</h4>
              <div
                v-for="n in detail.notices"
                :key="n.id"
                class="flex items-center gap-2 border-b border-border py-1.5"
              >
                <div class="flex-1">{{ n.content }}</div>
                <span class="whitespace-nowrap text-xs text-muted-foreground">{{ new Date(n.created_at).toLocaleDateString() }}</span>
                <Button v-if="isCaptain" size="sm" variant="ghost" class="text-destructive" @click="delNotice(n.id)">删</Button>
              </div>
              <Empty v-if="detail.notices.length === 0" description="暂无公告" class="py-4" />
              <div v-if="isCaptain" class="mt-2 flex gap-2">
                <Input v-model="noticeContent" class="flex-1" placeholder="发布队内公告…" @keyup.enter="postNotice" />
                <Button @click="postNotice">发布</Button>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent class="p-6">
              <h4 class="mb-3 text-base font-semibold">队内排行（按共享题单 AC 数）</h4>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead class="w-12">#</TableHead>
                    <TableHead>成员</TableHead>
                    <TableHead class="w-20">AC</TableHead>
                    <TableHead class="w-24">尝试题数</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="(row, i) in board" :key="row.user_id">
                    <TableCell>{{ i + 1 }}</TableCell>
                    <TableCell>
                      <router-link :to="`/users/${row.user_id}`">{{ row.nickname || row.username }}</router-link>
                    </TableCell>
                    <TableCell>{{ row.ac }}</TableCell>
                    <TableCell>{{ row.tried }}</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>
    </template>
  </div>
</template>
