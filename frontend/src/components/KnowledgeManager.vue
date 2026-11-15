<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'

const bases = ref([])
const selectedBaseId = ref('')
const documents = ref([])
const versions = ref([])
const loading = ref(false)
const saving = ref(false)
const uploading = ref(false)
const publishing = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const newName = ref('')
const newDescription = ref('')
const editName = ref('')
const editDescription = ref('')
const editEnabled = ref(true)
const selectedFiles = ref([])
const fileInput = ref(null)
const versionTitle = ref('')
const versionDescription = ref('')

const selectedBase = computed(() => bases.value.find((item) => item.id === selectedBaseId.value) || null)

function assertSuccess(result) {
  const code = Number(result?.code)
  if (result?.error || (Number.isFinite(code) && code !== 0 && code !== 200)) {
    throw new Error(result?.msg || '请求失败')
  }
  return result?.data ?? {}
}

function clearMessages() {
  errorMessage.value = ''
  successMessage.value = ''
}

async function loadBases() {
  loading.value = true
  try {
    const data = assertSuccess(await api.get('/admin/knowledge/bases'))
    bases.value = Array.isArray(data) ? data : []
    if (!bases.value.some((item) => item.id === selectedBaseId.value)) {
      selectedBaseId.value = bases.value[0]?.id || ''
    }
    if (selectedBaseId.value) await loadSelectedBase()
    else {
      documents.value = []
      versions.value = []
    }
  } catch (error) {
    errorMessage.value = error?.message || '读取知识库失败'
  } finally {
    loading.value = false
  }
}

async function loadSelectedBase() {
  const base = selectedBase.value
  if (!base) return
  editName.value = base.name || ''
  editDescription.value = base.description || ''
  editEnabled.value = Boolean(base.enabled)
  const [documentData, versionData] = await Promise.all([
    api.get('/admin/knowledge/documents', { base_id: base.id }),
    api.get('/admin/knowledge/versions', { base_id: base.id }),
  ])
  const documentItems = assertSuccess(documentData)
  const versionItems = assertSuccess(versionData)
  documents.value = Array.isArray(documentItems) ? documentItems : []
  versions.value = Array.isArray(versionItems) ? versionItems : []
}

async function selectBase(id) {
  selectedBaseId.value = id
  clearMessages()
  try {
    await loadSelectedBase()
  } catch (error) {
    errorMessage.value = error?.message || '读取知识库内容失败'
  }
}

async function createBase() {
  if (!newName.value.trim()) {
    errorMessage.value = '请先填写知识库名称。'
    return
  }
  saving.value = true
  clearMessages()
  try {
    const data = assertSuccess(await api.post('/admin/knowledge/bases/create', {
      name: newName.value.trim(),
      description: newDescription.value.trim(),
    }))
    newName.value = ''
    newDescription.value = ''
    await loadBases()
    if (data.id) await selectBase(data.id)
    successMessage.value = '知识库已创建。'
  } catch (error) {
    errorMessage.value = error?.message || '创建知识库失败'
  } finally {
    saving.value = false
  }
}

async function saveBaseSettings() {
  if (!selectedBase.value || !editName.value.trim()) return
  saving.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/admin/knowledge/bases/update', {
      id: selectedBase.value.id,
      name: editName.value.trim(),
      description: editDescription.value.trim(),
      enabled: editEnabled.value,
    }))
    successMessage.value = '知识库设置已保存。'
    await loadBases()
  } catch (error) {
    errorMessage.value = error?.message || '保存知识库设置失败'
  } finally {
    saving.value = false
  }
}

async function deleteBase() {
  if (!selectedBase.value) return
  if (!window.confirm('确定删除知识库“' + selectedBase.value.name + '”及其中的全部文档和版本吗？此操作不可撤销。')) return
  saving.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/admin/knowledge/bases/delete', { id: selectedBase.value.id }))
    selectedBaseId.value = ''
    successMessage.value = '知识库已删除。'
    await loadBases()
  } catch (error) {
    errorMessage.value = error?.message || '删除知识库失败'
  } finally {
    saving.value = false
  }
}

function handleFileChange(event) {
  selectedFiles.value = Array.from(event.target.files || [])
}

async function uploadFiles() {
  if (!selectedBase.value || !selectedFiles.value.length) {
    errorMessage.value = '请选择需要上传的文档。'
    return
  }
  uploading.value = true
  clearMessages()
  try {
    const body = new FormData()
    body.append('base_id', selectedBase.value.id)
    selectedFiles.value.forEach((file) => body.append('files', file))
    const data = assertSuccess(await api.post('/admin/knowledge/documents/upload', body))
    const count = Array.isArray(data) ? data.length : selectedFiles.value.length
    successMessage.value = '已完成文档解析和索引，共处理 ' + count + ' 个文件。'
    selectedFiles.value = []
    if (fileInput.value) fileInput.value.value = ''
    await loadBases()
  } catch (error) {
    errorMessage.value = error?.message || '上传文档失败'
  } finally {
    uploading.value = false
  }
}

async function deleteDocument(document) {
  if (!window.confirm('确定从知识库中删除“' + document.filename + '”吗？')) return
  saving.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/admin/knowledge/documents/delete', { id: document.id }))
    successMessage.value = '文档已删除。'
    await loadBases()
  } catch (error) {
    errorMessage.value = error?.message || '删除文档失败'
  } finally {
    saving.value = false
  }
}

async function publishVersion() {
  if (!selectedBase.value || documents.value.length === 0) {
    errorMessage.value = '知识库中至少需要有一个文档才能发布版本。'
    return
  }
  publishing.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/admin/knowledge/versions/publish', {
      base_id: selectedBase.value.id,
      title: versionTitle.value.trim(),
      description: versionDescription.value.trim(),
    }))
    versionTitle.value = ''
    versionDescription.value = ''
    successMessage.value = '知识库新版本已发布。'
    await loadSelectedBase()
  } catch (error) {
    errorMessage.value = error?.message || '发布知识库版本失败'
  } finally {
    publishing.value = false
  }
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  return (size / (1024 * 1024)).toFixed(1) + ' MB'
}

function formatDate(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

onMounted(loadBases)
</script>

<template>
  <div class="knowledge-manager">
    <section class="admin-card">
      <div class="admin-card-heading">
        <div>
          <h2>创建知识库</h2>
          <p>创建后上传文档，平台会在本地 SQLite 中保存文本并建立检索索引。</p>
        </div>
      </div>
      <form class="knowledge-create-form" @submit.prevent="createBase">
        <label>知识库名称
          <input v-model="newName" maxlength="100" placeholder="例如：学校规章制度" required />
        </label>
        <label>知识库说明
          <input v-model="newDescription" maxlength="500" placeholder="简要说明该知识库包含的内容" />
        </label>
        <button class="admin-primary" type="submit" :disabled="saving">创建知识库</button>
      </form>
    </section>

    <p v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</p>
    <p v-if="successMessage" class="admin-alert admin-alert-success" role="status">{{ successMessage }}</p>
    <p v-if="loading" class="admin-empty">正在读取知识库…</p>

    <div v-else class="knowledge-layout">
      <aside class="knowledge-sidebar admin-card">
        <div class="admin-card-heading">
          <div><h2>知识库列表</h2><p>{{ bases.length }} 个知识库</p></div>
          <button class="admin-secondary" type="button" :disabled="saving" @click="loadBases">刷新</button>
        </div>
        <button v-for="base in bases" :key="base.id" type="button"
          class="knowledge-list-item" :class="{ active: base.id === selectedBaseId }"
          @click="selectBase(base.id)">
          <strong>{{ base.name }}</strong>
          <small>{{ base.document_count }} 份文档 · {{ base.enabled ? '已启用' : '已停用' }}</small>
        </button>
        <p v-if="!bases.length" class="admin-empty">还没有知识库，请先创建。</p>
      </aside>

      <section class="knowledge-content admin-card">
        <template v-if="selectedBase">
          <div class="admin-card-heading">
            <div>
              <h2>{{ selectedBase.name }}</h2>
              <p>{{ selectedBase.description || '尚未填写知识库说明' }}</p>
            </div>
            <span class="admin-mode">{{ selectedBase.enabled ? '对话可用' : '已停用' }}</span>
          </div>

          <div class="knowledge-base-settings">
            <label>知识库名称<input v-model="editName" maxlength="100" /></label>
            <label>知识库说明<input v-model="editDescription" maxlength="500" /></label>
            <label class="knowledge-enabled-option"><input v-model="editEnabled" type="checkbox" /> 允许用户在对话中使用</label>
            <div class="admin-row-actions">
              <button class="admin-primary" type="button" :disabled="saving" @click="saveBaseSettings">保存设置</button>
              <button class="admin-text-button danger" type="button" :disabled="saving" @click="deleteBase">删除知识库</button>
            </div>
          </div>

          <div class="knowledge-upload">
            <label class="knowledge-file-picker">
              选择文档
              <input ref="fileInput" type="file" multiple accept=".txt,.md,.csv,.json,.html,.htm,.pdf,.docx" :disabled="uploading" @change="handleFileChange" />
            </label>
            <span>{{ selectedFiles.length ? selectedFiles.map((file) => file.name).join('、') : '支持 TXT、MD、CSV、JSON、HTML、PDF、DOCX，单次最多 20 个文件' }}</span>
            <button class="admin-primary" type="button" :disabled="uploading || !selectedFiles.length" @click="uploadFiles">
              {{ uploading ? '正在解析并索引…' : '上传并建立索引' }}
            </button>
          </div>

          <div class="admin-table-wrap">
            <table class="admin-table">
              <thead><tr><th>文档名称</th><th>大小</th><th>文本片段</th><th>上传时间</th><th>操作</th></tr></thead>
              <tbody>
                <tr v-for="document in documents" :key="document.id">
                  <td>{{ document.filename }}</td>
                  <td>{{ formatBytes(document.size_bytes) }}</td>
                  <td>{{ document.chunk_count }}</td>
                  <td>{{ formatDate(document.created_at) }}</td>
                  <td><button class="admin-text-button danger" type="button" :disabled="saving" @click="deleteDocument(document)">删除</button></td>
                </tr>
                <tr v-if="!documents.length"><td colspan="5" class="admin-empty">暂无文档，请上传需要检索的资料。</td></tr>
              </tbody>
            </table>
          </div>

          <div class="knowledge-version-panel">
            <div class="admin-card-heading">
              <div><h2>版本发布</h2><p>发布当前文档清单快照，便于记录知识库更新批次。</p></div>
            </div>
            <form class="knowledge-create-form" @submit.prevent="publishVersion">
              <label>版本名称<input v-model="versionTitle" maxlength="80" placeholder="留空自动生成版本号" /></label>
              <label>版本说明<input v-model="versionDescription" maxlength="500" placeholder="例如：2026 年秋季制度更新" /></label>
              <button class="admin-primary" type="submit" :disabled="publishing || !documents.length">{{ publishing ? '发布中…' : '发布新版本' }}</button>
            </form>
            <div class="admin-table-wrap">
              <table class="admin-table">
                <thead><tr><th>版本</th><th>名称</th><th>文档数</th><th>发布时间</th></tr></thead>
                <tbody>
                  <tr v-for="version in versions" :key="version.id">
                    <td>v{{ version.version_number }}</td>
                    <td>{{ version.title }}</td>
                    <td>{{ version.document_count }}</td>
                    <td>{{ formatDate(version.published_at) }}</td>
                  </tr>
                  <tr v-if="!versions.length"><td colspan="4" class="admin-empty">尚未发布版本。</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>
        <div v-else class="knowledge-unavailable">
          <div class="knowledge-empty-icon">文</div>
          <h2>选择或创建一个知识库</h2>
          <p>上传的文档会在本平台本地解析、分段并建立关键词检索索引。启用的知识库会出现在对话页面的知识库开关中。</p>
        </div>
      </section>
    </div>
  </div>
</template>
