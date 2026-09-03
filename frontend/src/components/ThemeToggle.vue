<script setup lang="ts">
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme, type Theme } from '@/composables/useTheme'

const { theme, setTheme } = useTheme()

const options: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: '浅色', icon: Sun },
  { value: 'dark', label: '暗色', icon: Moon },
  { value: 'system', label: '跟随系统', icon: Monitor },
]
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" class="relative" aria-label="切换主题">
        <Sun class="size-4 rotate-0 scale-100 transition-transform dark:rotate-90 dark:scale-0" />
        <Moon class="absolute size-4 rotate-90 scale-0 transition-transform dark:rotate-0 dark:scale-100" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem
        v-for="opt in options"
        :key="opt.value"
        @click="setTheme(opt.value)"
      >
        <component :is="opt.icon" class="size-4" />
        <span>{{ opt.label }}</span>
        <span v-if="theme === opt.value" class="ml-auto text-primary">✓</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
