<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Auth, errMsg } from '../api/client'
import { useAuthStore } from '../stores/auth'

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
    ElMessage.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

async function doRegister() {
  loading.value = true
  try {
    await Auth.register(regForm)
    ElMessage.success('注册成功，请登录')
    mode.value = 'login'
    loginForm.username = regForm.username
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div style="display: flex; justify-content: center; align-items: center; height: 100vh">
    <el-card style="width: min(400px, 92vw)">
      <h2 style="text-align: center">YSNB OJ</h2>
      <el-tabs v-model="mode">
        <el-tab-pane label="登录" name="login">
          <el-form @submit.prevent="doLogin">
            <el-form-item>
              <el-input v-model="loginForm.username" placeholder="用户名" />
            </el-form-item>
            <el-form-item>
              <el-input v-model="loginForm.password" type="password" placeholder="密码" show-password />
            </el-form-item>
            <el-button type="primary" native-type="submit" :loading="loading" style="width: 100%">
              登录
            </el-button>
          </el-form>
        </el-tab-pane>
        <el-tab-pane label="注册" name="register">
          <el-form @submit.prevent="doRegister">
            <el-form-item>
              <el-input v-model="regForm.username" placeholder="用户名（3-32 位）" />
            </el-form-item>
            <el-form-item>
              <el-input v-model="regForm.nickname" placeholder="昵称（可选）" />
            </el-form-item>
            <el-form-item>
              <el-input v-model="regForm.student_no" placeholder="学号（必填）" />
            </el-form-item>
            <el-form-item>
              <el-input v-model="regForm.password" type="password" placeholder="密码（至少 6 位）" show-password />
            </el-form-item>
            <el-form-item>
              <el-input v-model="regForm.invite_code" placeholder="邀请码（向管理员获取）" />
            </el-form-item>
            <el-button type="primary" native-type="submit" :loading="loading" style="width: 100%">
              注册
            </el-button>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>
