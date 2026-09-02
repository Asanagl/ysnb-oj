<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DataLine,
  Document,
  Trophy,
  User,
  Monitor,
  Key,
  Back,
  Connection,
} from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const items = computed(() => {
  const base = [
    { path: '/admin', label: '数据总览', icon: DataLine, tier: 'admin' },
    { path: '/admin/review', label: '题目审核', icon: Document, tier: 'staff' },
    { path: '/admin/problems', label: '题目管理', icon: Document, tier: 'staff' },
    { path: '/admin/contests', label: '比赛管理', icon: Trophy, tier: 'staff' },
    { path: '/admin/users', label: '用户管理', icon: User, tier: 'admin' },
    { path: '/admin/daemons', label: '判题机监控', icon: Monitor, tier: 'admin' },
    { path: '/admin/api-keys', label: 'API 密钥', icon: Key, tier: 'admin' },
    { path: '/admin/plugins', label: '插件系统', icon: Connection, tier: 'staff' },
  ]
  // tier: staff = setter 及以上可见；admin = admin 及以上可见
  return base.filter((i) => (i.tier === 'staff' ? auth.canManage : auth.isAdmin))
})

const pageTitle = computed(() => {
  const current = items.value.find((i) => i.path === route.path)
  return current?.label ?? (route.path.startsWith('/admin') ? '后台管理' : '')
})

function backToSite() {
  router.push('/')
}
function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <el-container style="min-height: 100vh">
    <el-aside width="220px" class="admin-aside">
      <div class="admin-logo">OJ 后台管理</div>
      <el-menu
        :default-active="$route.path"
        router
        background-color="#1d2b3a"
        text-color="#cfd8e3"
        active-text-color="#409eff"
        style="border-right: none"
      >
        <el-menu-item v-for="item in items" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </el-menu-item>
      </el-menu>
      <div style="flex: 1" />
      <div class="admin-aside-footer">
        <el-button :icon="Back" text style="color: #cfd8e3" @click="backToSite">
          返回前台
        </el-button>
      </div>
    </el-aside>

    <el-container>
      <el-header
        height="56px"
        class="admin-header"
      >
        <span style="font-size: 16px; font-weight: 600">{{ pageTitle }}</span>
        <span style="flex: 1" />
        <el-dropdown @command="logout">
          <span style="cursor: pointer">{{ auth.user?.nickname || auth.user?.username }}</span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="out">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main style="background: #f5f7fa; padding: 16px 20px">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.admin-aside {
  background: #1d2b3a;
  display: flex;
  flex-direction: column;
  position: sticky;
  top: 0;
  height: 100vh;
}
.admin-logo {
  color: #fff;
  font-weight: 700;
  font-size: 16px;
  padding: 20px 16px;
}
.admin-aside-footer {
  padding: 12px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}
.admin-header {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
}
@media (max-width: 991.98px) {
  .admin-aside {
    width: 64px !important;
  }
  .admin-logo,
  .admin-aside-footer {
    display: none;
  }
}
</style>
