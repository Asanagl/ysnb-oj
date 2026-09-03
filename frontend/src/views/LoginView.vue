<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Auth, errMsg } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import ThemeToggle from '@/components/ThemeToggle.vue'

const mode = ref<'login' | 'register'>('login')
const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)

const loginForm = reactive({ username: '', password: '' })
const regForm = reactive({ username: '', password: '', nickname: '', student_no: '', invite_code: '' })

async function doLogin() {
  loading.value = true
  try {
    const r = await Auth.login(loginForm.username, loginForm.password)
    auth.setSession(r.token, r.user)
    router.push('/')
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

async function doRegister() {
  loading.value = true
  try {
    await Auth.register(regForm)
    toast.success('注册成功，请登录')
    mode.value = 'login'
    loginForm.username = regForm.username
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-screen items-center justify-center bg-background">
    <div class="absolute top-4 right-4">
      <ThemeToggle />
    </div>
    <Card class="w-[min(400px,92vw)]">
      <CardHeader>
        <CardTitle class="text-center text-xl">
          YSNB <span class="text-primary">OJ</span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Tabs v-model="mode">
          <TabsList class="mb-4 w-full">
            <TabsTrigger value="login" class="flex-1">登录</TabsTrigger>
            <TabsTrigger value="register" class="flex-1">注册</TabsTrigger>
          </TabsList>
          <TabsContent value="login">
            <form class="flex flex-col gap-3" @submit.prevent="doLogin">
              <Input v-model="loginForm.username" placeholder="用户名" />
              <Input v-model="loginForm.password" type="password" placeholder="密码" />
              <Button type="submit" class="w-full" :disabled="loading">
                {{ loading ? '登录中…' : '登录' }}
              </Button>
            </form>
          </TabsContent>
          <TabsContent value="register">
            <form class="flex flex-col gap-3" @submit.prevent="doRegister">
              <Input v-model="regForm.username" placeholder="用户名（3-32 位）" />
              <Input v-model="regForm.nickname" placeholder="昵称（可选）" />
              <Input v-model="regForm.student_no" placeholder="学号（必填）" />
              <Input v-model="regForm.password" type="password" placeholder="密码（至少 6 位）" />
              <Input v-model="regForm.invite_code" placeholder="邀请码（向管理员获取）" />
              <Button type="submit" class="w-full" :disabled="loading">
                {{ loading ? '注册中…' : '注册' }}
              </Button>
            </form>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  </div>
</template>
