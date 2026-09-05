<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LogOut, Menu as MenuIcon } from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'
import { useResponsive } from '../composables/useResponsive'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import ThemeToggle from '@/components/ThemeToggle.vue'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { isPhone } = useResponsive()
const drawerOpen = ref(false)

const navItems = [
  { path: '/', label: '首页' },
  { path: '/problems', label: '题库' },
  { path: '/submissions', label: '提交记录' },
  { path: '/contests', label: '比赛' },
  { path: '/lists', label: '题单' },
  { path: '/teams', label: '小组' },
  { path: '/external', label: '刷题数据' },
  { path: '/profile', label: '个人中心' },
]

function isActive(path: string) {
  return path === '/' ? route.path === '/' : route.path.startsWith(path)
}

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
  <div class="min-h-screen bg-background">
    <header
      class="sticky top-0 z-40 flex h-14 items-center gap-6 border-b border-border bg-card px-4 md:px-8"
    >
      <router-link to="/" class="text-[17px] font-extrabold tracking-wide whitespace-nowrap">
        YSNB <span class="text-primary">OJ</span>
      </router-link>

      <nav v-if="!isPhone" class="flex flex-1 items-center gap-1">
        <router-link
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          :class="
            cn(
              'rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
              isActive(item.path) && 'bg-accent-soft font-semibold text-primary',
            )
          "
        >
          {{ item.label }}
        </router-link>
        <router-link
          v-if="auth.canManage"
          to="/admin"
          :class="
            cn(
              'rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
              route.path.startsWith('/admin') && 'bg-accent-soft font-semibold text-primary',
            )
          "
        >
          后台管理
        </router-link>
      </nav>
      <span v-else class="flex-1" />

      <ThemeToggle />

      <DropdownMenu v-if="auth.logged && !isPhone">
        <DropdownMenuTrigger as-child>
          <Button variant="ghost" class="px-2">
            {{ auth.user?.nickname || auth.user?.username }}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem danger @click="logout">
            <LogOut class="size-4" />
            退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Button
        v-if="isPhone"
        variant="ghost"
        size="icon"
        aria-label="打开菜单"
        @click="drawerOpen = true"
      >
        <MenuIcon class="size-5" />
      </Button>
    </header>

    <Sheet v-model:open="drawerOpen">
      <SheetContent side="left" class="flex w-72 flex-col p-4">
        <div class="px-2 pb-4 text-base font-extrabold">
          YSNB <span class="text-primary">OJ</span>
        </div>
        <nav class="flex flex-col gap-1">
          <router-link
            v-for="item in navItems"
            :key="item.path"
            :to="item.path"
            :class="
              cn(
                'rounded-md px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
                isActive(item.path) && 'bg-accent-soft font-semibold text-primary',
              )
            "
          >
            {{ item.label }}
          </router-link>
          <router-link
            v-if="auth.canManage"
            to="/admin"
            :class="
              cn(
                'rounded-md px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
                route.path.startsWith('/admin') && 'bg-accent-soft font-semibold text-primary',
              )
            "
          >
            后台管理
          </router-link>
        </nav>
        <div class="flex-1" />
        <Button v-if="auth.logged" variant="outline" class="w-full" @click="logout">
          退出登录
        </Button>
      </SheetContent>
    </Sheet>

    <main>
      <div class="oj-page">
        <router-view />
      </div>
    </main>
  </div>
</template>
