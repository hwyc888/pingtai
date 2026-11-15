import { createRouter, createWebHistory } from 'vue-router'
import { hasAccessToken } from './auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/IntelligentAI' },
    { path: '/login', name: 'login', component: () => import('./views/LoginView.vue') },
    { path: '/IntelligentAI', name: 'chat', component: () => import('./views/ChatView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/login' },
  ],
})

router.beforeEach((to) => {
  const signedIn = hasAccessToken()
  if (to.path === '/login' && signedIn) return '/IntelligentAI'
  if (to.path !== '/login' && !signedIn) return '/login'
  return true
})

export default router
