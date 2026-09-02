<script setup lang="ts">
// 小组详情：成员列表、队内公告、共享题单、队内排行；队长管理面板。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Teams, type TeamDetail, type TeamBoardRow } from '../api/client'

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
  try {
    const r = await Teams.leave(teamId.value)
    if ((r as { deleted?: boolean }).deleted) {
      ElMessage.success('小组已解散')
      router.push('/teams')
    } else {
      ElMessage.success('已退出')
      router.push('/teams')
    }
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
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
      ElMessage.error('找不到该用户名')
      return
    }
    await Teams.pull(teamId.value, u.id)
    ElMessage.success('已拉入')
    pullName.value = ''
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function kick(uid: number) {
  try {
    await Teams.kick(teamId.value, uid)
    ElMessage.success('已移除')
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function transfer(uid: number) {
  try {
    await Teams.transfer(teamId.value, uid)
    ElMessage.success('队长已转让')
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function regenInvite() {
  try {
    await Teams.regenerateInvite(teamId.value)
    ElMessage.success('邀请码已重置')
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function postNotice() {
  if (!noticeContent.value.trim()) return
  try {
    await Teams.createNotice(teamId.value, noticeContent.value)
    noticeContent.value = ''
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function delNotice(nid: number) {
  try {
    await Teams.deleteNotice(teamId.value, nid)
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function share() {
  if (!shareListId.value) return
  try {
    await Teams.shareList(teamId.value, shareListId.value)
    ElMessage.success('题单已共享')
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}

async function unshare(listId: number) {
  try {
    await Teams.unshareList(teamId.value, listId)
    await load()
  } catch (e) {
    ElMessage.error((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? String(e))
  }
}
</script>

<template>
  <div v-loading="loading">
    <template v-if="detail">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; flex-wrap: wrap; gap: 8px">
        <div>
          <h2 style="margin: 0">{{ detail.team.name }}</h2>
          <p style="color: #909399; margin: 6px 0 0">{{ detail.team.bio }}</p>
        </div>
        <div style="display: flex; gap: 8px">
          <el-tag v-if="detail.team.role === 'captain'" type="warning">队长</el-tag>
          <el-tag v-else-if="detail.team.role === 'member'" type="success">队员</el-tag>
          <el-popconfirm v-if="detail.team.role !== 'none'" title="确定退出小组？" @confirm="leave">
            <template #reference><el-button type="danger" text>退出小组</el-button></template>
          </el-popconfirm>
        </div>
      </div>

      <el-row :gutter="16">
        <!-- 成员 + 队长管理 -->
        <el-col :xs="24" :md="10">
          <el-card shadow="never" style="margin-bottom: 16px">
            <h4 style="margin-top: 0">成员（{{ detail.team.member_count }}/{{ detail.team.capacity }}）</h4>
            <div v-for="m in detail.team.members" :key="m.user_id" class="tm-row">
              <router-link :to="`/users/${m.user_id}`" style="flex: 1">
                {{ m.nickname || m.username }}
                <el-tag v-if="m.is_captain" size="small" type="warning">队长</el-tag>
              </router-link>
              <template v-if="isCaptain && !m.is_captain">
                <el-button size="small" text @click="transfer(m.user_id)">转让队长</el-button>
                <el-popconfirm :title="`移除 ${m.nickname || m.username}？`" @confirm="kick(m.user_id)">
                  <template #reference><el-button size="small" type="danger" text>移除</el-button></template>
                </el-popconfirm>
              </template>
            </div>
            <template v-if="isCaptain">
              <el-divider />
              <div style="display: flex; gap: 8px; margin-bottom: 8px">
                <el-input v-model="pullName" size="small" placeholder="按用户名拉人" @keyup.enter="pull" />
                <el-button size="small" @click="pull">拉人</el-button>
              </div>
              <div style="display: flex; gap: 8px; align-items: center">
                <code style="flex: 1">{{ detail.team.invite_code }}</code>
                <el-button size="small" @click="regenInvite">重置邀请码</el-button>
              </div>
            </template>
          </el-card>

          <el-card shadow="never" style="margin-bottom: 16px">
            <h4 style="margin-top: 0">共享题单</h4>
            <div v-for="l in detail.lists" :key="l.id" class="tm-row">
              <router-link :to="`/lists/${l.id}`" style="flex: 1">{{ l.title }}（{{ l.items }} 题）</router-link>
              <el-button v-if="isCaptain" size="small" type="danger" text @click="unshare(l.id)">取消共享</el-button>
            </div>
            <el-empty v-if="detail.lists.length === 0" description="还没有共享题单" :image-size="50" />
            <template v-if="isCaptain">
              <div style="display: flex; gap: 8px; margin-top: 8px">
                <el-select v-model="shareListId" placeholder="选择我的题单" size="small" style="flex: 1">
                  <el-option v-for="l in myLists" :key="l.id" :label="l.title" :value="l.id" />
                </el-select>
                <el-button size="small" type="primary" @click="share">共享</el-button>
              </div>
            </template>
          </el-card>
        </el-col>

        <!-- 公告 + 排行 -->
        <el-col :xs="24" :md="14">
          <el-card shadow="never" style="margin-bottom: 16px">
            <h4 style="margin-top: 0">队内公告</h4>
            <div v-for="n in detail.notices" :key="n.id" class="tm-notice">
              <div style="flex: 1">{{ n.content }}</div>
              <span style="color: #c0c4cc; font-size: 12px; white-space: nowrap">{{ new Date(n.created_at).toLocaleDateString() }}</span>
              <el-button v-if="isCaptain" size="small" type="danger" text @click="delNotice(n.id)">删</el-button>
            </div>
            <el-empty v-if="detail.notices.length === 0" description="暂无公告" :image-size="50" />
            <div v-if="isCaptain" style="display: flex; gap: 8px; margin-top: 8px">
              <el-input v-model="noticeContent" placeholder="发布队内公告…" @keyup.enter="postNotice" />
              <el-button type="primary" @click="postNotice">发布</el-button>
            </div>
          </el-card>

          <el-card shadow="never">
            <h4 style="margin-top: 0">队内排行（按共享题单 AC 数）</h4>
            <el-table :data="board" size="small">
              <el-table-column label="#" type="index" width="50" />
              <el-table-column label="成员" min-width="140">
                <template #default="{ row }">
                  <router-link :to="`/users/${row.user_id}`">{{ row.nickname || row.username }}</router-link>
                </template>
              </el-table-column>
              <el-table-column label="AC" prop="ac" width="80" />
              <el-table-column label="尝试题数" prop="tried" width="90" />
            </el-table>
          </el-card>
        </el-col>
      </el-row>
    </template>
  </div>
</template>

<style scoped>
.tm-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 0;
  border-bottom: 1px solid #f0f2f5;
}
.tm-notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid #f0f2f5;
}
</style>
