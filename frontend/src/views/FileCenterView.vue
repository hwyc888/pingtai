<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'

const props = defineProps({ embedded: { type: Boolean, default: false } })
const router = useRouter()
const loading = ref(false)
const uploading = ref(false)
const isAdmin = ref(false)
const knowledgeBases = ref([])
const selectedBaseId = ref('')
const query = ref('')
const extension = ref('all')
const ownerFilter = ref('')
const files = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const selectedFiles = ref([])
const accessMode = ref('private')
const dragActive = ref(false)
const fileInput = ref(null)
const errorMessage = ref('')
const successMessage = ref('')
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function unwrap(result) {
  if (result?.error || (Number.isFinite(Number(result?.code)) && ![0, 200].includes(Number(result.code)))) throw new Error(result?.msg || '请求失败')
  return result?.data ?? result ?? {}
}
function formatBytes(value) {
  const size = Number(value || 0)
  if (size < 1024) return size + ' B'
  if (size < 1048576) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1073741824) return (size / 1048576).toFixed(2) + ' MB'
  return (size / 1073741824).toFixed(2) + ' GB'
}
function formatDate(value) {
  const date = new Date(value)
  return !value || Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}
function iconFor(file) {
  const ext = String(file.extension || '').toLowerCase()
  if (ext === '.pdf') return 'PDF'
  if (['.doc', '.docx'].includes(ext)) return 'DOC'
  if (['.xls', '.xlsx', '.csv'].includes(ext)) return 'XLS'
  if (['.ppt', '.pptx'].includes(ext)) return 'PPT'
  if (['.png', '.jpg', '.jpeg', '.webp'].includes(ext)) return 'IMG'
  return 'TXT'
}
function fileURL(file, action) {
  const base = (window.g?.baseURL || '/api').replace(/\/$/, '')
  return base + '/files/' + encodeURIComponent(file.id) + '/' + action
}
function snippetParts(value) {
  const text = String(value || '')
  const term = query.value.trim()
  const index = term ? text.toLocaleLowerCase().indexOf(term.toLocaleLowerCase()) : -1
  if (index < 0) return [{ text, matched: false }]
  return [
    { text: text.slice(0, index), matched: false },
    { text: text.slice(index, index + term.length), matched: true },
    { text: text.slice(index + term.length), matched: false },
  ]
}
async function loadProfile() {
  const result = await api.get('/auth/me')
  isAdmin.value = Boolean((result?.data ?? result ?? {}).is_admin)
}

async function loadKnowledgeBases() {
  const path = isAdmin.value ? '/knowledge/manage/bases' : '/knowledge/my-bases'
  const result = unwrap(await api.get(path))
  knowledgeBases.value = Array.isArray(result) ? result : []
  if (!knowledgeBases.value.some((base) => base.id === selectedBaseId.value)) {
    selectedBaseId.value = ''
  }
}
async function searchFiles(nextPage = 1) {
  loading.value = true
  errorMessage.value = ''
  page.value = nextPage
  try {
    const params = { q: query.value.trim(), type: extension.value, page: page.value, page_size: pageSize }
    if (isAdmin.value && ownerFilter.value.trim()) params.owner = ownerFilter.value.trim()
    const result = unwrap(await api.get('/files/search', params))
    files.value = Array.isArray(result.items) ? result.items : []
    total.value = Number(result.total || 0)
  } catch (error) {
    files.value = []
    total.value = 0
    errorMessage.value = error?.message || '检索文件失败'
  } finally { loading.value = false }
}
function addFiles(list) {
  const merged = [...selectedFiles.value]
  for (const file of Array.from(list || [])) {
    if (!merged.some((item) => item.name === file.name && item.size === file.size && item.lastModified === file.lastModified)) merged.push(file)
  }
  selectedFiles.value = merged.slice(0, 20)
  if (merged.length > 20) errorMessage.value = '单次最多选择 20 个文件。'
}
function onFileChange(event) {
  addFiles(event.target.files)
  event.target.value = ''
}
function onDrop(event) {
  dragActive.value = false
  addFiles(event.dataTransfer?.files)
}
async function uploadFiles() {
  if (!selectedFiles.value.length || uploading.value) return
  uploading.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const body = new FormData()
    body.append('access_mode', accessMode.value)
    selectedFiles.value.forEach((file) => body.append('files', file))
    const result = unwrap(await api.post('/files/upload', body))
    const uploaded = result.items || []
    const failed = [...(result.failed || [])]
    let attached = 0
    if (selectedBaseId.value) {
      for (const file of uploaded) {
        if (!file.id) continue
        try {
          await api.post('/knowledge/manage/documents/attach-file', {
            base_id: selectedBaseId.value,
            file_id: file.id,
          })
          attached += 1
        } catch (error) {
          failed.push({ name: file.name || '文件', error: error?.message || '加入知识库失败' })
        }
      }
    }
    selectedFiles.value = []
    const baseName = knowledgeBases.value.find((base) => base.id === selectedBaseId.value)?.name
    successMessage.value = '已保存 ' + uploaded.length + ' 个文件' +
      (attached ? '，并已关联到知识库「' + (baseName || '所选知识库') + '」' : '') +
      (failed.length ? '，' + failed.length + ' 个项目需要检查' : '') + '。'
    if (failed.length) errorMessage.value = failed.map((item) => item.name + '：' + item.error).join('；')
    await searchFiles(1)
    if (attached) await loadKnowledgeBases()
  } catch (error) {
    errorMessage.value = error?.message || '上传文件失败'
  } finally { uploading.value = false }
}
async function attachFile(file) {
  if (!selectedBaseId.value) {
    errorMessage.value = '请先在上传区域选择目标知识库。'
    return
  }
  try {
    unwrap(await api.post('/knowledge/manage/documents/attach-file', {
      base_id: selectedBaseId.value,
      file_id: file.id,
    }))
    successMessage.value = '「' + file.name + '」已加入所选知识库，对话检索现在可以使用这份资料。'
    await Promise.all([searchFiles(page.value), loadKnowledgeBases()])
  } catch (error) {
    errorMessage.value = error?.message || '加入知识库失败'
  }
}

async function deleteFile(file) {
  if (file.knowledge_bases) {
    errorMessage.value = '该文件仍被知识库引用，请先从知识库中移除文档关联。'
    return
  }
  if (!window.confirm('确定删除「' + file.name + '」吗？原始文件也会从服务器移除。')) return
  try {
    unwrap(await api.post('/files/' + encodeURIComponent(file.id) + '/delete', {}))
    successMessage.value = '文件已删除。'
    await searchFiles(Math.min(page.value, totalPages.value))
  } catch (error) { errorMessage.value = error?.message || '删除文件失败' }
}
async function initialize() {
  try { await loadProfile() } catch {}
  try { await loadKnowledgeBases() } catch {}
  await searchFiles(1)
}
onMounted(initialize)
</script>

<template>
  <div class="file-center-page" :class="{ 'file-center-embedded': props.embedded }">
    <header v-if="!props.embedded" class="file-center-header">
      <div class="file-center-brand">
        <BrandMark class="file-center-brand-mark" />
        <div><p class="file-center-eyebrow">超凡 AI 问答平台 · 文件管理</p><h1>资料中心</h1><p>集中保存、检索和下载平台资料</p></div>
      </div>
      <button class="file-center-quiet-button" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
    </header>

    <section class="file-center-panel">
      <div class="file-center-panel-heading"><div><h2>搜索文件</h2><p>输入文件名称，或输入文档正文中的关键词</p></div><span>{{ total.toLocaleString() }} 份资料</span></div>
      <form class="file-center-search" @submit.prevent="searchFiles(1)">
        <label><span>⌕</span><input v-model="query" type="search" placeholder="搜索文件名或内容，例如：教学工作计划、学生管理制度" /><button v-if="query" type="button" @click="query = ''; searchFiles(1)">×</button></label>
        <select v-model="extension" aria-label="文件类型"><option value="all">全部类型</option><option value="pdf">PDF</option><option value="docx">Word</option><option value="xlsx">Excel</option><option value="pptx">PPT</option><option value="txt">TXT</option><option value="csv">CSV</option><option value="png">图片</option></select>
        <button class="file-center-primary-button" type="submit" :disabled="loading">搜索资料</button>
      </form>
      <div v-if="isAdmin" class="file-center-admin-filter"><label>上传者</label><input v-model="ownerFilter" placeholder="按上传者账号筛选" @keydown.enter.prevent="searchFiles(1)" /><button type="button" @click="searchFiles(1)">筛选</button></div>
    </section>

    <section class="file-center-panel file-center-upload" :class="{ dragging: dragActive }" @dragenter.prevent="dragActive = true" @dragover.prevent="dragActive = true" @dragleave.prevent="dragActive = false" @drop.prevent="onDrop">
      <div class="file-center-panel-heading"><div><h2>上传资料</h2><p>支持 PDF、Word、Excel、PPT、TXT、图片等常见文件，单批最多 20 个</p></div><span class="file-center-upload-symbol">＋</span></div>
      <div class="file-center-dropzone">
        <span class="file-center-upload-glyph">⇧</span><strong>拖拽文件到这里，或选择本地文件</strong><small>原始文件保存在服务器，后续可检索和下载</small>
        <input ref="fileInput" type="file" multiple accept=".pdf,.doc,.docx,.xlsx,.xls,.pptx,.ppt,.txt,.md,.csv,.json,.html,.htm,.png,.jpg,.jpeg,.webp" @change="onFileChange" />
        <button class="file-center-quiet-button" type="button" @click="fileInput?.click()">选择文件</button>
      </div>
      <div v-if="knowledgeBases.length" class="file-center-link-target">
        <label>目标知识库
          <select v-model="selectedBaseId">
            <option value="">仅保存为知识文件</option>
            <option v-for="base in knowledgeBases" :key="base.id" :value="base.id">{{ base.name }}</option>
          </select>
        </label>
        <small>选择知识库后，新上传文件会自动建立检索索引；也可将已有文件加入所选知识库。</small>
      </div>
      <div v-if="selectedFiles.length" class="file-center-pending">
        <div class="file-center-pending-heading">
          <strong>待上传 {{ selectedFiles.length }} 个文件</strong>
          <label>文件权限<select v-model="accessMode"><option value="private">仅自己和管理员可见</option><option value="shared">所有登录用户可见</option></select></label>
        </div>
        <div class="file-center-pending-list">
          <div v-for="(file,index) in selectedFiles" :key="file.name + file.size + file.lastModified" class="file-center-pending-item">
            <span>{{ file.name.split('.').pop().toUpperCase().slice(0,4) }}</span><strong :title="file.name">{{ file.name }}</strong><small>{{ formatBytes(file.size) }}</small><button type="button" @click="selectedFiles.splice(index,1)">×</button>
          </div>
        </div>
        <div class="file-center-upload-actions"><button class="file-center-quiet-button" type="button" @click="selectedFiles = []">清空</button><button class="file-center-primary-button" type="button" :disabled="uploading" @click="uploadFiles">{{ uploading ? '正在保存和建立索引…' : '上传并保存资料' }}</button></div>
      </div>
    </section>

    <p v-if="successMessage" class="file-center-notice success">{{ successMessage }}</p>
    <p v-if="errorMessage" class="file-center-notice error">{{ errorMessage }}</p>

    <section class="file-center-panel file-center-results">
      <div class="file-center-panel-heading"><div><h2>{{ query.trim() ? '检索结果' : '最近上传' }}</h2><p>{{ query.trim() ? '找到 ' + total.toLocaleString() + ' 份匹配资料' : '你有权限访问的最新资料' }}</p></div><button class="file-center-quiet-button" type="button" :disabled="loading" @click="searchFiles(1)">刷新</button></div>
      <div v-if="loading" class="file-center-empty">正在检索资料…</div>
      <div v-else-if="!files.length" class="file-center-empty"><span>⌕</span><strong>没有找到匹配的文件</strong><small>试试缩短关键词、切换类型，或先上传资料</small></div>
      <div v-else class="file-center-results-list">
        <article v-for="file in files" :key="file.id" class="file-center-result">
          <span class="file-center-type" :class="'type-' + iconFor(file).toLowerCase()">{{ iconFor(file) }}</span>
          <div class="file-center-result-main">
            <div class="file-center-result-title"><h3>{{ file.name }}</h3><span v-if="file.match_type === 'filename'" class="file-center-tag">文件名匹配</span><span v-else-if="file.match_type === 'content'" class="file-center-tag content">内容匹配</span><span v-if="file.access_mode === 'shared'" class="file-center-tag shared">共享</span></div>
            <p class="file-center-meta">{{ file.extension.toUpperCase().replace('.', '') }} · {{ formatBytes(file.size_bytes) }} · 上传者：{{ file.owner_username }} · {{ formatDate(file.created_at) }}</p>
            <p v-if="file.knowledge_bases" class="file-center-linked-bases">已关联知识库：{{ file.knowledge_bases }}</p>
            <p v-if="file.snippet" class="file-center-snippet"><template v-for="(part,index) in snippetParts(file.snippet)" :key="index"><mark v-if="part.matched">{{ part.text }}</mark><span v-else>{{ part.text }}</span></template></p>
            <p v-if="file.extraction_status !== 'indexed'" class="file-center-extract-note">当前仅支持文件名检索：{{ file.extraction_error || '该格式暂未配置全文提取' }}</p>
          </div>
          <div class="file-center-actions">
            <a v-if="file.can_preview" :href="fileURL(file,'preview')" target="_blank" rel="noopener">预览</a>
            <a :href="fileURL(file,'download')" download>下载原文件</a>
            <button v-if="selectedBaseId" type="button" @click="attachFile(file)">加入所选知识库</button>
            <button v-if="file.can_delete" type="button" @click="deleteFile(file)">删除</button>
            <small v-else-if="file.knowledge_bases">已被知识库引用</small>
          </div>
        </article>
      </div>
      <div v-if="totalPages > 1" class="file-center-pagination"><button :disabled="page <= 1 || loading" @click="searchFiles(page - 1)">上一页</button><span>第 {{ page }} / {{ totalPages }} 页</span><button :disabled="page >= totalPages || loading" @click="searchFiles(page + 1)">下一页</button></div>
    </section>
    <footer class="file-center-footer">原始文件与内容索引分开保存；删除资料会同时移除服务器原文件。</footer>
  </div>
</template>
