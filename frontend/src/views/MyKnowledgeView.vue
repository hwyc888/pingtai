<script setup>
import { useRouter } from 'vue-router'
import KnowledgeManager from '../components/KnowledgeManager.vue'

const router = useRouter()
</script>

<template>
  <main class="admin-page knowledge-admin-page">
    <header class="admin-header">
      <div>
        <h1>我的知识库</h1>
        <p class="admin-subtitle">普通账号只管理本人归属的知识库；管理员可在此查看全部知识库并配置公共、指定用户或私有权限。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
      </div>
    </header>
    <KnowledgeManager :admin-mode="false" />
  </main>
</template>
