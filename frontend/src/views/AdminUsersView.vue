<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Admin, errMsg } from '../api/client'
import { useAuthStore } from '../stores/auth'

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
    ElMessage.success(`导入成功 ${r.created} 人`)
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

function openEdit(row: { id: number; username: string; student_no: string; nickname: string }) {
  Object.assign(editForm, { id: row.id, username: row.username, student_no: row.student_no ?? '', nickname: row.nickname ?? '' })
  editDialog.value = true
}

async function saveEdit() {
  try {
    await Admin.updateUser(editForm.id, editForm.student_no, editForm.nickname)
    ElMessage.success('已保存')
    editDialog.value = false
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

async function doResetPassword(row: { id: number; username: string }) {
  try {
    const r = await Admin.resetPassword(row.id)
    newPasswords.value.push({ username: row.username, password: r.password })
    ElMessage.success('密码已重置，见右侧弹窗')
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// 角色修改：admin 走 /role（user↔setter），super_admin 走 /role/super
async function onRoleChange(row: { id: number; username: string; role: string }, role: string) {
  if (role === row.role) return
  const action = auth.isSuperAdmin ? () => Admin.setSuperRole(row.id, role) : () => Admin.setRole(row.id, role)
  try {
    await action()
    ElMessage.success(`已将 ${row.username} 的角色改为 ${roleLabel(role)}`)
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
    await load()
  }
}

async function onBanToggle(row: { id: number; username: string; banned: boolean }) {
  const next = !row.banned
  try {
    await Admin.setBan(row.id, next)
    ElMessage.success(next ? `已封禁 ${row.username}` : `已解封 ${row.username}`)
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

async function createInvite() {
  try {
    await Admin.createInvite(inviteForm.value)
    ElMessage.success('邀请码已生成')
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// 删除邀请码：只影响后续注册，已注册账号不受影响（提示里已说明）
async function deleteInvite(row: { id: number; code: string }) {
  try {
    await Admin.deleteInvite(row.id)
    ElMessage.success(`邀请码 ${row.code} 已删除`)
    await load()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
</script>

<template>
  <el-card>
    <h3>用户管理</h3>
    <el-row :gutter="16">
      <el-col :span="12">
        <h4>批量导入（CSV: username,student_no,nickname,password）</h4>
        <input type="file" accept=".csv" @change="onImport" />
        <el-table v-if="importRows.length" :data="importRows" size="small" style="margin-top: 8px">
          <el-table-column label="用户名" prop="username" />
          <el-table-column label="初始密码" prop="password" />
          <el-table-column label="错误" prop="err" />
        </el-table>
      </el-col>
      <el-col :span="12">
        <h4>邀请码</h4>
        <el-form inline>
          <el-form-item label="可用次数">
            <el-input-number v-model="inviteForm.max_uses" :min="1" :max="999" />
          </el-form-item>
          <el-form-item label="有效小时">
            <el-input-number v-model="inviteForm.expires_in_hours" :min="0" :max="720" />
          </el-form-item>
          <el-button type="primary" @click="createInvite">生成</el-button>
        </el-form>
        <el-table :data="invites" size="small">
          <el-table-column label="邀请码" prop="code" />
          <el-table-column label="已用/上限">
            <template #default="{ row }">{{ row.used_count }}/{{ row.max_uses }}</template>
          </el-table-column>
          <el-table-column label="过期时间">
            <template #default="{ row }">
              {{ row.expires_at ? new Date(row.expires_at).toLocaleString() : '不限' }}
            </template>
          </el-table-column>
          <el-table-column label="操作" width="80">
            <template #default="{ row }">
              <el-popconfirm
                title="删除该邀请码？已注册用户不受影响"
                width="240"
                @confirm="deleteInvite(row)"
              >
                <template #reference>
                  <el-button size="small" type="danger" text>删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-col>
    </el-row>
    <h4 style="margin-top: 16px">全部用户（{{ users.length }}）</h4>
    <el-table :data="users" size="small">
      <el-table-column label="ID" prop="id" width="70" />
      <el-table-column label="用户名" prop="username" />
      <el-table-column label="昵称" prop="nickname" />
      <el-table-column label="学号" prop="student_no" />
      <el-table-column label="角色" width="180">
        <template #default="{ row }">
          <el-select
            :model-value="row.role"
            size="small"
            :disabled="row.id === auth.user?.id"
            @change="(v: string) => onRoleChange(row, v)"
          >
            <!-- 现角色始终在选项里（即使超出操作者权限范围），避免显示错乱 -->
            <el-option
              v-for="opt in [...roleOptions.filter((o) => o.value === row.role), ...roleOptions]"
              :key="opt.value + (opt.value === row.role ? '-cur' : '')"
              :label="opt.value === row.role ? `${roleLabel(row.role)}（当前）` : opt.label"
              :value="opt.value"
              :disabled="opt.value === row.role"
            />
          </el-select>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.banned" type="danger" size="small">已封禁</el-tag>
          <el-tag v-else type="success" size="small">正常</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-popconfirm title="重置该用户密码？" @confirm="doResetPassword(row)">
            <template #reference>
              <el-button size="small" type="warning">重置密码</el-button>
            </template>
          </el-popconfirm>
          <el-popconfirm
            :title="row.banned ? `解封 ${row.username}？` : `封禁 ${row.username}？封禁后立即无法登录`"
            width="240"
            @confirm="onBanToggle(row)"
          >
            <template #reference>
              <el-button size="small" :type="row.banned ? 'success' : 'danger'">
                {{ row.banned ? '解封' : '封禁' }}
              </el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog :model-value="newPasswords.length > 0" title="重置后的密码" width="420px" @update:model-value="newPasswords = []">
      <div v-for="p in newPasswords" :key="p.username" style="margin-bottom: 8px">
        <strong>{{ p.username }}</strong>：<code>{{ p.password }}</code>
      </div>
      <p style="color: #909399; font-size: 12px">请立即复制分发，此弹窗关闭后不再显示</p>
    </el-dialog>

    <el-dialog v-model="editDialog" title="编辑用户" width="420px">
      <el-form label-width="80px">
        <el-form-item label="用户名">
          <el-input :model-value="editForm.username" disabled />
        </el-form-item>
        <el-form-item label="学号">
          <el-input v-model="editForm.student_no" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="editForm.nickname" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialog = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>
