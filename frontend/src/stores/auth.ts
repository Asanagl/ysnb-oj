import { defineStore } from 'pinia'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('oj_token') ?? '',
    user: JSON.parse(localStorage.getItem('oj_user') ?? 'null') as
      | { id: number; username: string; nickname: string; role: string }
      | null,
  }),
  getters: {
    logged: (s) => !!s.token,
    isSuperAdmin: (s) => s.user?.role === 'super_admin',
    isAdmin: (s) => s.user?.role === 'admin' || s.user?.role === 'super_admin',
    canManage: (s) => s.user?.role === 'admin' || s.user?.role === 'setter' || s.user?.role === 'super_admin',
  },
  actions: {
    setSession(token: string, user: { id: number; username: string; nickname: string; role: string }) {
      this.token = token
      this.user = user
      localStorage.setItem('oj_token', token)
      localStorage.setItem('oj_user', JSON.stringify(user))
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem('oj_token')
      localStorage.removeItem('oj_user')
    },
  },
})
