<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Admin, errMsg } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NumberInput } from '@/components/ui/number-input'
import { Badge } from '@/components/ui/badge'
import { FormField } from '@/components/ui/form-field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const auth = useAuthStore()
const users = ref<Awaited<ReturnType<typeof Admin.users>>>([])
const invites = ref<Awaited<ReturnType<typeof Admin.invites>>>([])
const importRows = ref<{ username: string; password: string; err: string }[]>([])
const inviteForm = ref({ max_uses: 1, expires_in_hours: 72 })
const editDialog = ref(false)
const editForm = reactive({ id: 0, username: '', student_no: '', nickname: '' })
const newPasswords = ref<{ username: string; password: string }[]>([])

// 角色选项按操作者级别过滤：admin 只能改 user↔setter；
// super_admin 可授予/收回 admin 与 super_admin。
const roleOptions = auth.isSuperAdmin
  ? [
      { label: '普通用户 (user)', value: 'user' },
      { label: '出题人 (setter)', value: 'setter' },
      { label: '管理员 (admin)', value: 'admin' },
      { label: '超级管理员 (super_admin)', value: 'super_admin' },
    ]
  : [
      { label: '普通用户 (user)', value: 'user' },
      { label: '出题人 (setter)', value: 'setter' },
    ]
const roleLabel = (r: string) =>
  ({ super_admin: '超级管理员', admin: '管理员', setter: '出题人', user: '用户' })[r] ?? r

async function load() {
  users.value = await Admin.users('')
  invites.value = await Admin.invites()
}
onMounted(load)

async function onImport(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  try {
    const r = await Admin.importUsers(file)
    importRows.value = r.rows
    toast.success(`导入成功 ${r.created} 人`)
    await load()
  } catch (err) {
    toast.error(errMsg(err))
  }
}

function openEdit(row: { id: number; username: string; student_no?: string; nickname: string }) {
  Object.assign(editForm, { id: row.id, username: row.username, student_no: row.student_no ?? '', nickname: row.nickname ?? '' })
  editDialog.value = true
}

async function saveEdit() {
  try {
    await Admin.updateUser(editForm.id, editForm.student_no, editForm.nickname)
    toast.success('已保存')
    editDialog.value = false
    await load()
  } catch (err) {
    toast.error(errMsg(err))
  }
}

async function doResetPassword(row: { id: number; username: string }) {
  if (!(await confirmDialog({ title: '重置该用户密码？', danger: true }))) return
  try {
    const r = await Admin.resetPassword(row.id)
    newPasswords.value.push({ username: row.username, password: r.password })
    toast.success('密码已重置，见右侧弹窗')
  } catch (err) {
    toast.error(errMsg(err))
  }
}

// 角色修改：admin 走 /role（user↔setter），super_admin 走 /role/super
async function onRoleChange(row: { id: number; username: string; role: string }, role: string) {
  if (role === row.role) return
  if (
    !(await confirmDialog({
      title: `将 ${row.username} 的角色改为 ${roleLabel(role)}？`,
      danger: true,
    }))
  )
    return
  const action = auth.isSuperAdmin ? () => Admin.setSuperRole(row.id, role) : () => Admin.setRole(row.id, role)
  try {
    await action()
    toast.success(`已将 ${row.username} 的角色改为 ${roleLabel(role)}`)
    await load()
  } catch (err) {
    toast.error(errMsg(err))
    await load()
  }
}

async function onBanToggle(row: { id: number; username: string; banned?: boolean }) {
  const next = !row.banned
  if (
    !(await confirmDialog({
      title: next ? `封禁 ${row.username}？` : `解封 ${row.username}？`,
      description: next ? '封禁后立即无法登录' : undefined,
      danger: true,
    }))
  )
    return
  try {
    await Admin.setBan(row.id, next)
    toast.success(next ? `已封禁 ${row.username}` : `已解封 ${row.username}`)
    await load()
  } catch (err) {
    toast.error(errMsg(err))
  }
}

async function createInvite() {
  try {
    await Admin.createInvite(inviteForm.value)
    toast.success('邀请码已生成')
    await load()
  } catch (err) {
    toast.error(errMsg(err))
  }
}

// 删除邀请码：只影响后续注册，已注册账号不受影响（提示里已说明）
async function deleteInvite(row: { id: number; code: string }) {
  if (
    !(await confirmDialog({
      title: '删除该邀请码？',
      description: '已注册用户不受影响',
      danger: true,
    }))
  )
    return
  try {
    await Admin.deleteInvite(row.id)
    toast.success(`邀请码 ${row.code} 已删除`)
    await load()
  } catch (err) {
    toast.error(errMsg(err))
  }
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>用户管理</CardTitle>
    </CardHeader>
    <CardContent>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <h4 class="mb-2 text-sm font-medium">批量导入（CSV: username,student_no,nickname,password）</h4>
          <input
            type="file"
            accept=".csv"
            class="text-sm file:mr-2 file:cursor-pointer file:rounded-md file:border file:border-input file:bg-card file:px-3 file:py-1.5 file:text-sm file:shadow-sm hover:file:bg-muted"
            @change="onImport"
          />
          <Table v-if="importRows.length" class="mt-2">
            <TableHeader>
              <TableRow>
                <TableHead>用户名</TableHead>
                <TableHead>初始密码</TableHead>
                <TableHead>错误</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="(row, i) in importRows" :key="i">
                <TableCell>{{ row.username }}</TableCell>
                <TableCell>{{ row.password }}</TableCell>
                <TableCell>{{ row.err }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
        <div>
          <h4 class="mb-2 text-sm font-medium">邀请码</h4>
          <div class="mb-3 flex flex-wrap items-end gap-3">
            <FormField label="可用次数" class="w-32">
              <NumberInput v-model="inviteForm.max_uses" :min="1" :max="999" />
            </FormField>
            <FormField label="有效小时" class="w-32">
              <NumberInput v-model="inviteForm.expires_in_hours" :min="0" :max="720" />
            </FormField>
            <Button @click="createInvite">生成</Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>邀请码</TableHead>
                <TableHead>已用/上限</TableHead>
                <TableHead>过期时间</TableHead>
                <TableHead class="w-20">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="row in invites" :key="row.id">
                <TableCell>{{ row.code }}</TableCell>
                <TableCell>{{ row.used_count }}/{{ row.max_uses }}</TableCell>
                <TableCell>
                  {{ row.expires_at ? new Date(row.expires_at).toLocaleString() : '不限' }}
                </TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" class="text-destructive" @click="deleteInvite(row)">
                    删除
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>
      <h4 class="mb-2 mt-4 text-sm font-medium">全部用户（{{ users.length }}）</h4>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">ID</TableHead>
            <TableHead>用户名</TableHead>
            <TableHead>昵称</TableHead>
            <TableHead>学号</TableHead>
            <TableHead class="w-[180px]">角色</TableHead>
            <TableHead class="w-[90px]">状态</TableHead>
            <TableHead class="w-[240px]">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in users" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.username }}</TableCell>
            <TableCell>{{ row.nickname }}</TableCell>
            <TableCell>{{ row.student_no }}</TableCell>
            <TableCell>
              <Select
                :model-value="row.role"
                :disabled="row.id === auth.user?.id"
                @update:model-value="(v) => onRoleChange(row, String(v))"
              >
                <SelectTrigger class="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <!-- 现角色始终在选项里（即使超出操作者权限范围），避免显示错乱 -->
                  <SelectItem
                    v-for="opt in [...roleOptions.filter((o) => o.value === row.role), ...roleOptions]"
                    :key="opt.value + (opt.value === row.role ? '-cur' : '')"
                    :value="opt.value"
                    :disabled="opt.value === row.role"
                  >
                    {{ opt.value === row.role ? `${roleLabel(row.role)}（当前）` : opt.label }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </TableCell>
            <TableCell>
              <Badge v-if="row.banned" variant="destructive">已封禁</Badge>
              <Badge v-else variant="ac">正常</Badge>
            </TableCell>
            <TableCell>
              <div class="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" @click="openEdit(row)">编辑</Button>
                <Button variant="secondary" size="sm" @click="doResetPassword(row)">重置密码</Button>
                <Button
                  :variant="row.banned ? 'secondary' : 'destructive'"
                  size="sm"
                  :class="row.banned ? 'text-ac' : ''"
                  @click="onBanToggle(row)"
                >
                  {{ row.banned ? '解封' : '封禁' }}
                </Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <Dialog :open="newPasswords.length > 0" @update:open="newPasswords = []">
        <DialogContent class="max-w-[420px]">
          <DialogHeader>
            <DialogTitle>重置后的密码</DialogTitle>
          </DialogHeader>
          <div v-for="p in newPasswords" :key="p.username" class="mb-2">
            <strong>{{ p.username }}</strong>：<code>{{ p.password }}</code>
          </div>
          <p class="text-xs text-muted-foreground">请立即复制分发，此弹窗关闭后不再显示</p>
        </DialogContent>
      </Dialog>

      <Dialog v-model:open="editDialog">
        <DialogContent class="max-w-[420px]">
          <DialogHeader>
            <DialogTitle>编辑用户</DialogTitle>
          </DialogHeader>
          <div class="flex flex-col gap-3">
            <FormField label="用户名">
              <Input :model-value="editForm.username" disabled />
            </FormField>
            <FormField label="学号">
              <Input v-model="editForm.student_no" />
            </FormField>
            <FormField label="昵称">
              <Input v-model="editForm.nickname" />
            </FormField>
          </div>
          <DialogFooter>
            <Button variant="outline" @click="editDialog = false">取消</Button>
            <Button @click="saveEdit">保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </CardContent>
  </Card>
</template>
