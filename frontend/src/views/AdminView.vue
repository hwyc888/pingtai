<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'
import ProviderManager from '../components/ProviderManager.vue'
import KnowledgeCenterView from './KnowledgeCenterView.vue'
import LoginSettings from '../components/LoginSettings.vue'

const router = useRouter()
const loading = ref(true)
const activeTab = ref('providers')
const errorMessage = ref('')

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const profile = await api.get('/auth/me')
    if (!profile?.data?.is_admin) errorMessage.value = '当前账号没有管理员权限。'
  } catch (error) {
    errorMessage.value = error?.message || '无法加载管理数据'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="admin-page" :class="{ 'knowledge-admin-page': activeTab === 'knowledge' }">
    <header class="admin-header">
      <div>
        <div class="admin-brand-line"><BrandMark class="admin-brand-mark" /><p class="admin-eyebrow">超凡AI问答平台 · 管理中心</p></div>
        <h1>平台管理</h1>
        <p class="admin-subtitle">管理模型供应商、知识库、资料文件和平台登录方式。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
      </div>
    </header>

    <p v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</p>
    <p v-if="loading" class="admin-empty">正在加载管理数据…</p>

    <template v-else-if="!errorMessage">
      <nav class="admin-tabs" aria-label="平台管理分类">
        <button type="button" :class="{ active: activeTab === 'providers' }" @click="activeTab = 'providers'">
          <span>◇</span> 模型供应商
        </button>
        <button type="button" :class="{ active: activeTab === 'knowledge' }" @click="activeTab = 'knowledge'">
          <span>▤</span> 知识中心
        </button>
        <button type="button" :class="{ active: activeTab === 'login' }" @click="activeTab = 'login'">
          <span>♙</span> 登录设置
        </button>
      </nav>

      <ProviderManager v-if="activeTab === 'providers'" />
      <KnowledgeCenterView v-else-if="activeTab === 'knowledge'" :admin-mode="true" embedded />
      <LoginSettings v-else />
    </template>
  </main>
</template>
