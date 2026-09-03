<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  ChartColumn,
  FileText,
  KeyRound,
  LogOut,
  Monitor,
  Trophy,
  Users,
  Webhook,
} from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import ThemeToggle from '@/components/ThemeToggle.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const items = computed(() => {
  const base = [
    { path: '/admin', label: '数据总览', icon: ChartColumn, tier: 'admin' },
    { path: '/admin/review', label: '题目审核', icon: FileText, tier: 'staff' },
    { path: '/admin/problems', label: '题目管理', icon: FileText, tier: 'staff' },
    { path: '/admin/contests', label: '比赛管理', icon: Trophy, tier: 'staff' },
    { path: '/admin/users', label: '用户管理', icon: Users, tier: 'admin' },
    { path: '/admin/daemons', label: '判题机监控', icon: Monitor, tier: 'admin' },
    { path: '/admin/api-keys', label: 'API 密钥', icon: KeyRound, tier: 'admin' },
    { path: '/admin/plugins', label: '插件系统', icon: Webhook, tier: 'staff' },
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
  <div class="flex min-h-screen bg-background">
    <!-- 992px 以下折叠为纯图标侧栏（原 991.98px 断点保留） -->
    <aside
      class="sticky top-0 flex h-screen w-16 flex-col border-r border-border bg-card min-[992px]:w-56"
    >
      <div class="px-4 py-5 text-base font-extrabold whitespace-nowrap max-[991.98px]:hidden">
        OJ <span class="text-primary">后台管理</span>
      </div>
      <nav class="flex flex-col gap-1 px-2">
        <router-link
          v-for="item in items"
          :key="item.path"
          :to="item.path"
          :title="item.label"
          :class="
            cn(
              'flex items-center gap-2.5 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
              route.path === item.path && 'bg-accent-soft font-semibold text-primary',
            )
          "
        >
          <component :is="item.icon" class="size-4 shrink-0" />
          <span class="max-[991.98px]:hidden">{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="flex-1" />
      <div class="border-t border-border p-2 max-[991.98px]:hidden">
        <Button variant="ghost" class="w-full justify-start" @click="backToSite">
          <ArrowLeft class="size-4" />
          返回前台
        </Button>
      </div>
      <div class="border-t border-border p-2 min-[992px]:hidden">
        <Button variant="ghost" size="icon" class="w-full" title="返回前台" @click="backToSite">
          <ArrowLeft class="size-4" />
        </Button>
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col">
      <header
        class="flex h-14 items-center gap-3 border-b border-border bg-card px-4 md:px-6"
      >
        <span class="text-base font-semibold">{{ pageTitle }}</span>
        <span class="flex-1" />
        <ThemeToggle />
        <DropdownMenu>
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
      </header>
      <main class="flex-1 p-4 md:p-6">
        <router-view />
      </main>
    </div>
  </div>
</template>
