<script setup>
import { useRouter } from 'vue-router'
import KnowledgeManager from '../components/KnowledgeManager.vue'

const router = useRouter()
</script>

<template>
  <main class="admin-page">
    <header class="admin-header">
      <div>
        <h1>我的知识库</h1>
        <p class="admin-subtitle">这里只能管理当前账号创建的知识库；管理员可以查看和管理所有用户的知识库。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
      </div>
    </header>
    <KnowledgeManager />
  </main>
</template>
