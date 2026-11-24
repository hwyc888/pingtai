<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import BrandMark from '../components/BrandMark.vue'
import KnowledgeManager from '../components/KnowledgeManager.vue'
import FileCenterView from './FileCenterView.vue'

defineProps({
  adminMode: { type: Boolean, default: false },
  embedded: { type: Boolean, default: false },
})

const router = useRouter()
const activeTab = ref('bases')
</script>

<template>
  <component :is="embedded ? 'section' : 'main'" class="knowledge-center-view" :class="{ 'admin-page knowledge-admin-page': !embedded, 'knowledge-center-embedded': embedded }">
    <header v-if="!embedded" class="admin-header">
      <div>
        <div class="admin-brand-line"><BrandMark class="admin-brand-mark" /><p class="admin-eyebrow">超凡 AI 问答平台 · 统一资料管理</p></div>
        <h1 class="knowledge-page-title"><img src="/zhishiku.png" alt="" aria-hidden="true" />知识中心</h1>
        <p class="admin-subtitle">统一管理知识库和原始知识文件；上传到知识库的文档也会自动归档到知识文件。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
      </div>
    </header>

    <nav class="knowledge-center-tabs" aria-label="知识中心分类">
      <button type="button" :class="{ active: activeTab === 'bases' }" @click="activeTab = 'bases'">
        <img src="/zhishiku.png" alt="" aria-hidden="true" />
        <span>知识库</span>
        <small>管理知识集合与 AI 检索</small>
      </button>
      <button type="button" :class="{ active: activeTab === 'files' }" @click="activeTab = 'files'">
        <span class="knowledge-center-file-icon">▤</span>
        <span>知识文件</span>
        <small>统一搜索、预览和下载原文件</small>
      </button>
    </nav>

    <KnowledgeManager v-if="activeTab === 'bases'" :admin-mode="adminMode" />
    <FileCenterView v-else embedded />
  </component>
</template>
