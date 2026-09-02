<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu as MenuIcon } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useResponsive } from '../composables/useResponsive'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { isPhone } = useResponsive()
const drawerOpen = ref(false)

// why close on route change: drawer nav stays open after picking an item
// otherwise, and the tap target for "close" is small on phones.
watch(
  () => route.fullPath,
  () => (drawerOpen.value = false),
)

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <el-container style="min-height: 100vh">
    <el-header
      height="56px"
      style="background: #1d2b3a; display: flex; align-items: center; gap: 8px; padding: 0 16px"
    >
      <span style="color: #fff; font-weight: 700; font-size: 17px; white-space: nowrap">
        <span v-if="!isPhone" style="margin-right: 24px">YSNB OJ</span>
        <span v-else>OJ</span>
      </span>

      <template v-if="!isPhone">
        <el-menu
          mode="horizontal"
          background-color="#1d2b3a"
          text-color="#cfd8e3"
          active-text-color="#409eff"
          :default-active="$route.path"
          router
          style="flex: 1; border-bottom: none"
        >
          <el-menu-item index="/">首页</el-menu-item>
          <el-menu-item index="/problems">题库</el-menu-item>
          <el-menu-item index="/submissions">提交记录</el-menu-item>
          <el-menu-item index="/contests">比赛</el-menu-item>
          <el-menu-item index="/lists">题单</el-menu-item>
          <el-menu-item index="/teams">小组</el-menu-item>
          <el-menu-item index="/profile">个人中心</el-menu-item>
          <el-menu-item v-if="auth.canManage" index="/admin">后台管理</el-menu-item>
        </el-menu>
      </template>
      <template v-else>
        <span style="flex: 1" />
        <el-icon
          style="color: #fff; font-size: 24px; cursor: pointer; padding: 8px"
          @click="drawerOpen = true"
        >
          <MenuIcon />
        </el-icon>
      </template>

      <el-dropdown v-if="auth.logged && !isPhone" @command="logout">
        <span style="color: #fff; cursor: pointer">
          {{ auth.user?.nickname || auth.user?.username }}
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="out">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </el-header>

    <el-drawer v-model="drawerOpen" direction="ltr" size="72%" :with-header="false">
      <div style="display: flex; flex-direction: column; height: 100%">
        <div style="font-weight: 700; font-size: 16px; padding: 4px 8px 16px">
          YSNB OJ
        </div>
        <el-menu :default-active="$route.path" router style="border-right: none">
          <el-menu-item index="/">首页</el-menu-item>
          <el-menu-item index="/problems">题库</el-menu-item>
          <el-menu-item index="/submissions">提交记录</el-menu-item>
          <el-menu-item index="/contests">比赛</el-menu-item>
          <el-menu-item index="/lists">题单</el-menu-item>
          <el-menu-item index="/teams">小组</el-menu-item>
          <el-menu-item index="/profile">个人中心</el-menu-item>
          <el-menu-item v-if="auth.canManage" index="/admin">后台管理</el-menu-item>
        </el-menu>
        <div style="flex: 1" />
        <el-button v-if="auth.logged" style="margin: 12px" @click="logout">退出登录</el-button>
      </div>
    </el-drawer>

    <el-main style="padding: 0">
      <div class="oj-page">
        <router-view />
      </div>
    </el-main>
  </el-container>
</template>
