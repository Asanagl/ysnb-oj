import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    {
      // fullscreen projection scoreboard: no nav chrome by design
      path: '/contests/:id/board',
      component: () => import('../views/BoardView.vue'),
    },
    {
      // standalone admin console with its own sidebar layout
      path: '/admin',
      component: () => import('../layouts/AdminLayout.vue'),
      children: [
        { path: '', component: () => import('../views/AdminDashboardView.vue'), meta: { roles: ['admin', 'super_admin'] } },
        { path: 'review', component: () => import('../views/AdminReviewView.vue'), meta: { roles: ['admin', 'setter', 'super_admin'] } },
        { path: 'problems', component: () => import('../views/AdminProblemsView.vue'), meta: { roles: ['admin', 'setter', 'super_admin'] } },
        { path: 'contests', component: () => import('../views/AdminContestsView.vue'), meta: { roles: ['admin', 'setter', 'super_admin'] } },
        { path: 'users', component: () => import('../views/AdminUsersView.vue'), meta: { roles: ['admin', 'super_admin'] } },
        { path: 'daemons', component: () => import('../views/AdminDaemonsView.vue'), meta: { roles: ['admin', 'super_admin'] } },
        { path: 'api-keys', component: () => import('../views/AdminApiKeysView.vue'), meta: { roles: ['admin', 'super_admin'] } },
        { path: 'plugins', component: () => import('../views/AdminPluginsView.vue'), meta: { roles: ['admin', 'setter', 'super_admin'] } },
      ],
    },
    {
      // standalone full-page problem editor (bigger viewport for authors)
      path: '/problems/new',
      component: () => import('../views/ProblemEditorView.vue'),
    },
    {
      path: '/problems/:id/edit',
      component: () => import('../views/ProblemEditorView.vue'),
    },
    {
      // same editor for the user-created review flow (my/problems scope)
      path: '/my/problems/new',
      component: () => import('../views/MyProblemEditorView.vue'),
    },
    {
      path: '/my/problems/:id/edit',
      component: () => import('../views/MyProblemEditorView.vue'),
    },
    {
      path: '/',
      component: () => import('../layouts/MainLayout.vue'),
      children: [
        { path: '', component: () => import('../views/HomeView.vue') },
        { path: 'problems', component: () => import('../views/ProblemsView.vue') },
        { path: 'problems/:id', component: () => import('../views/ProblemDetailView.vue') },
        { path: 'submissions', component: () => import('../views/SubmissionsView.vue') },
        { path: 'submissions/:id', component: () => import('../views/SubmissionDetailView.vue') },
        { path: 'contests', component: () => import('../views/ContestsView.vue') },
        { path: 'contests/:id', component: () => import('../views/ContestDetailView.vue') },
        {
          path: 'contests/:cid/problems/:pid',
          component: () => import('../views/ContestProblemView.vue'),
        },
        { path: 'profile', component: () => import('../views/ProfileView.vue') },
        { path: 'external', component: () => import('../views/ExternalPracticeView.vue') },
        { path: 'my/problems', component: () => import('../views/MyProblemsView.vue') },
        { path: 'users/:id', component: () => import('../views/ProfileView.vue') },
        { path: 'lists', component: () => import('../views/ListsView.vue') },
        { path: 'lists/new', component: () => import('../views/ListEditorView.vue') },
        { path: 'lists/:id', component: () => import('../views/ListDetailView.vue') },
        { path: 'lists/:id/edit', component: () => import('../views/ListEditorView.vue') },
        { path: 'teams', component: () => import('../views/TeamsView.vue') },
        { path: 'teams/:id', component: () => import('../views/TeamDetailView.vue') },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const logged = !!localStorage.getItem('oj_token')
  if (!to.meta.public && !logged) return '/login'
  if (to.path === '/login' && logged) return '/'
  // role gate for admin-console routes (meta.roles: ['admin'] etc.)
  const roles = to.meta.roles as string[] | undefined
  if (roles) {
    const user = JSON.parse(localStorage.getItem('oj_user') ?? 'null') as
      | { role?: string }
      | null
    if (!user?.role || !roles.includes(user.role)) return '/'
  }
  return true
})

export default router
