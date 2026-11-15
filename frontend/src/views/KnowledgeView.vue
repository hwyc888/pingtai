<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'

const router = useRouter()
const loading = ref(true)
const saving = ref(false)
const available = ref(false)
const workspaceId = ref('')
const datasets = ref([])
const selectedDataset = ref(null)
const documents = ref([])
const versions = ref([])
const errorMessage = ref('')
const successMessage = ref('')
const datasetName = ref('')
const datasetDescription = ref('')
const uploadFiles = ref([])
const filePage = ref(1)
const fileTotal = ref(0)
const versionTitle = ref('')
const versionDescription = ref('')
const expandedVersionId = ref(0)

const selectedTitle = computed(() => selectedDataset.value?.name || selectedDataset.value?.dataset_name || '选择知识库')
const canPublishVersion = computed(() => documents.value.length > 0 && documents.value.every((item) => Number(item.status) === 1))

function unwrap(result) {
  if (result?.error || (Number.isFinite(Number(result?.code)) && ![0, 200].includes(Number(result.code)))) {
    throw new Error(result?.msg || '请求失败')
  }
  return result?.data ?? result ?? {}
}

function listFrom(data, keys) {
  if (Array.isArray(data)) return data
  for (const key of keys) if (Array.isArray(data?.[key])) return data[key]
  if (Array.isArray(data?.data)) return data.data
  return []
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const profile = unwrap(await api.get('/auth/me'))
    if (!profile.is_admin) throw new Error('仅管理员可以管理知识库。')
    const status = unwrap(await api.get('/datasets/status'))
    available.value = Boolean(status.available)
    workspaceId.value = status.workspace_id || ''
    if (available.value) await loadDatasets()
  } catch (error) {
    errorMessage.value = error?.message || '知识库初始化失败'
  } finally {
    loading.value = false
  }
}

async function loadDatasets() {
  if (!workspaceId.value) throw new Error('Coze 工作空间 ID 未配置。')
  const result = unwrap(await api.get('/datasets/knowledgeList', {
    space_id: workspaceId.value, page_num: 1, page_size: 100,
  }))
  datasets.value = listFrom(result, ['datasets', 'list', 'items'])
  if (selectedDataset.value) {
    selectedDataset.value = datasets.value.find((item) =>
      datasetID(item) === datasetID(selectedDataset.value)
    ) || null
  }
  if (!selectedDataset.value && datasets.value.length) {
    await selectDataset(datasets.value[0])
  } else if (!datasets.value.length) {
    documents.value = []
    versions.value = []
  }
}

function datasetID(item = selectedDataset.value) {
  return item?.dataset_id || item?.id || ''
}

async function createDataset() {
  if (!datasetName.value.trim()) {
    errorMessage.value = '请输入知识库名称。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    unwrap(await api.post('/datasets/createKnowledge', {
      space_id: workspaceId.value,
      format_type: 0,
      name: datasetName.value.trim(),
      description: datasetDescription.value.trim(),
    }))
    successMessage.value = '知识库创建请求已提交。'
    datasetName.value = ''
    datasetDescription.value = ''
    await loadDatasets()
  } catch (error) {
    errorMessage.value = error?.message || '创建知识库失败'
  } finally {
    saving.value = false
  }
}

async function selectDataset(item) {
  selectedDataset.value = item
  documents.value = []
  versions.value = []
  filePage.value = 1
  expandedVersionId.value = 0
  await Promise.all([loadDocuments(), loadVersions()])
}

async function loadDocuments() {
  if (!datasetID()) return
  saving.value = true
  errorMessage.value = ''
  try {
    const result = unwrap(await api.post('/datasets/knowledgeFileList', {
      dataset_id: datasetID(),
      page_num: filePage.value,
      page_size: 50,
    }))
    documents.value = listFrom(result, ['documents', 'document_infos', 'list', 'items'])
    fileTotal.value = Number(result.total || result.total_count || documents.value.length)
  } catch (error) {
    errorMessage.value = error?.message || '读取知识库文档失败'
  } finally {
    saving.value = false
  }
}

async function loadVersions() {
  if (!datasetID()) {
    versions.value = []
    return
  }
  try {
    const result = unwrap(await api.get('/datasets/versions', { dataset_id: datasetID() }))
    versions.value = listFrom(result, ['versions', 'items', 'list'])
  } catch (error) {
    errorMessage.value = error?.message || '读取知识库版本失败'
  }
}

function documentStatus(item) {
  const status = Number(item?.status ?? item?.process_status)
  if (status === 0) return '处理中'
  if (status === 1) return '已完成'
  if (status === 9) return '处理失败'
  return '未知'
}

function formatDate(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function toggleVersion(id) {
  expandedVersionId.value = expandedVersionId.value === Number(id) ? 0 : Number(id)
}

async function publishVersion() {
  if (!datasetID()) {
    errorMessage.value = '请先选择知识库。'
    return
  }
  if (!canPublishVersion.value) {
    errorMessage.value = '请先上传文档，并等待所有文档处理完成后再发布。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const nextNumber = Math.max(0, ...versions.value.map((item) => Number(item.version_number || 0))) + 1
    const result = unwrap(await api.post('/datasets/versions/publish', {
      dataset_id: datasetID(),
      title: versionTitle.value.trim() || '版本 ' + nextNumber,
      description: versionDescription.value.trim(),
    }))
    successMessage.value = '知识库版本 v' + result.version_number + ' 已发布。'
    versionTitle.value = ''
    versionDescription.value = ''
    await loadVersions()
  } catch (error) {
    errorMessage.value = error?.message || '发布知识库版本失败'
  } finally {
    saving.value = false
  }
}

function onFilesChanged(event) {
  uploadFiles.value = Array.from(event.target.files || [])
}

async function uploadDocuments() {
  if (!datasetID() || !uploadFiles.value.length) {
    errorMessage.value = '请选择知识库和要上传的文件。'
    return
  }
  const files = uploadFiles.value.slice()
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    for (let offset = 0; offset < files.length; offset += 10) {
      const form = new FormData()
      form.append('dataset_id', datasetID())
      form.append('format_type', '0')
      form.append('source', 'file')
      for (const file of files.slice(offset, offset + 10)) form.append('files', file)
      unwrap(await api.post('/datasets/createKnowledgeFile', form))
    }
    successMessage.value = '已提交 ' + files.length + ' 个文档，正在等待知识库处理。'
    uploadFiles.value = []
    const input = document.getElementById('knowledge-files')
    if (input) input.value = ''
    await loadDocuments()
  } catch (error) {
    errorMessage.value = error?.message || '上传知识库文档失败；已提交的批次不会自动撤销'
    await loadDocuments()
  } finally {
    saving.value = false
  }
}

async function deleteDocument(item) {
  const id = item.document_id || item.id
  if (!id || !window.confirm('确定从知识库删除该文档吗？')) return
  saving.value = true
  try {
    unwrap(await api.post('/datasets/deleteKnowledgeFile', { document_ids: [id] }))
    successMessage.value = '文档删除请求已提交。请重新发布版本以记录本次变更。'
    await loadDocuments()
  } catch (error) {
    errorMessage.value = error?.message || '删除文档失败'
  } finally {
    saving.value = false
  }
}

async function deleteDataset(item) {
  const id = datasetID(item)
  if (!id || !window.confirm('确定删除整个知识库及其文档吗？此操作不可撤销。')) return
  saving.value = true
  try {
    unwrap(await api.post('/datasets/delKnowledge', { dataset_id: id }))
    selectedDataset.value = null
    documents.value = []
    versions.value = []
    successMessage.value = '知识库删除请求已提交。'
    await loadDatasets()
  } catch (error) {
    errorMessage.value = error?.message || '删除知识库失败'
  } finally {
    saving.value = false
  }
}

async function changePage(delta) {
  const pageCount = Math.max(1, Math.ceil(fileTotal.value / 50))
  const nextPage = Math.min(pageCount, Math.max(1, filePage.value + delta))
  if (nextPage === filePage.value) return
  filePage.value = nextPage
  await loadDocuments()
}

async function refresh() {
  if (selectedDataset.value) await Promise.all([loadDocuments(), loadVersions()])
  else await loadDatasets()
}

onMounted(load)
</script>

<template>
  <main class="knowledge-page">
    <header class="knowledge-header">
      <div>
        <div class="knowledge-brand-line"><BrandMark class="knowledge-brand-mark" /><p class="admin-eyebrow">超凡AI问答平台 · 知识库</p></div>
        <h1>知识库管理</h1>
        <p class="admin-subtitle">管理知识库、上传资料、跟踪处理状态并发布版本。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
        <button class="admin-secondary" type="button" @click="refresh" :disabled="saving">刷新</button>
      </div>
    </header>

    <p v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</p>
    <p v-if="successMessage" class="admin-alert admin-alert-success" role="status">{{ successMessage }}</p>
    <p v-if="loading" class="admin-empty">正在加载知识库…</p>

    <section v-else-if="!available" class="admin-card knowledge-unavailable">
      <BrandMark class="knowledge-empty-icon" />
      <h2>知识库服务尚未连接</h2>
      <p>当前知识库管理使用 Coze 知识库 API，需要配置 Coze Access Token，或 Coze OAuth 应用凭据。</p>
      <p>普通 OpenAI 兼容模型 API Key 不等于知识库凭据，不能直接用于文档存储和索引。</p>
      <button class="admin-primary" type="button" @click="router.push('/setup')">前往系统设置</button>
    </section>

    <template v-else>
      <section class="admin-card">
        <div class="admin-card-heading">
          <div><h2>创建知识库</h2><p>在当前 Coze 工作空间下创建文本知识库。</p></div>
        </div>
        <form class="knowledge-create-form" @submit.prevent="createDataset">
          <label>知识库名称<input v-model="datasetName" maxlength="100" placeholder="例如：产品使用手册" required /></label>
          <label>描述<input v-model="datasetDescription" maxlength="500" placeholder="简要说明知识库内容" /></label>
          <button class="admin-primary" type="submit" :disabled="saving">创建知识库</button>
        </form>
      </section>

      <div class="knowledge-layout">
        <section class="admin-card knowledge-sidebar">
          <div class="admin-card-heading"><div><h2>知识库列表</h2><p>{{ datasets.length }} 个知识库</p></div></div>
          <button v-for="item in datasets" :key="datasetID(item)" type="button"
            class="knowledge-list-item" :class="{ active: datasetID(item) === datasetID() }"
            @click="selectDataset(item)">
            <strong>{{ item.name || item.dataset_name || '未命名知识库' }}</strong>
            <small>{{ item.description || datasetID(item) }}</small>
          </button>
          <p v-if="!datasets.length" class="admin-empty">暂无知识库</p>
        </section>

        <section class="admin-card knowledge-content">
          <div class="admin-card-heading">
            <div><h2>{{ selectedTitle }}</h2><p>上传资料、查看处理状态和维护文档。</p></div>
            <button v-if="selectedDataset" class="admin-text-button danger" type="button" @click="deleteDataset(selectedDataset)">删除知识库</button>
          </div>
          <template v-if="selectedDataset">
            <div class="knowledge-upload">
              <label class="knowledge-file-picker">选择文件<input id="knowledge-files" type="file" multiple accept=".txt,.pdf,.doc,.docx" @change="onFilesChanged" /></label>
              <span>{{ uploadFiles.length ? uploadFiles.map((file) => file.name).join('、') : '支持 TXT、PDF、DOC、DOCX；每批最多 10 个' }}</span>
              <button class="admin-primary" type="button" :disabled="saving || !uploadFiles.length" @click="uploadDocuments">上传文档</button>
            </div>
            <div class="admin-table-wrap">
              <table class="admin-table knowledge-table">
                <thead><tr><th>文档名称</th><th>状态</th><th>分段</th><th>更新时间</th><th>操作</th></tr></thead>
                <tbody>
                  <tr v-for="item in documents" :key="item.document_id || item.id">
                    <td>{{ item.name || item.document_name || item.display_name || '未命名文档' }}</td>
                    <td><span class="knowledge-status" :class="'status-' + Number(item.status)">{{ documentStatus(item) }}</span></td>
                    <td>{{ item.slice_count ?? '-' }}</td>
                    <td>{{ formatDate(item.update_time || item.updated_at || item.create_time) }}</td>
                    <td><button class="admin-text-button danger" type="button" :disabled="saving" @click="deleteDocument(item)">删除</button></td>
                  </tr>
                  <tr v-if="!documents.length"><td colspan="5" class="admin-empty">该知识库暂无文档</td></tr>
                </tbody>
              </table>
            </div>
            <div v-if="fileTotal > 50" class="knowledge-pagination">
              <button class="admin-secondary" type="button" :disabled="filePage <= 1 || saving" @click="changePage(-1)">上一页</button>
              <span>第 {{ filePage }} / {{ Math.ceil(fileTotal / 50) }} 页，共 {{ fileTotal }} 个文档</span>
              <button class="admin-secondary" type="button" :disabled="filePage >= Math.ceil(fileTotal / 50) || saving" @click="changePage(1)">下一页</button>
            </div>

            <section class="knowledge-version-panel">
              <div class="admin-card-heading">
                <div><h2>版本发布</h2><p>所有文档处理完成后，可以将当前文档清单发布为一个版本。</p></div>
              </div>
              <div class="knowledge-version-form">
                <label>版本名称<input v-model="versionTitle" maxlength="80" placeholder="例如：2026 秋季资料版" /></label>
                <label>版本说明<input v-model="versionDescription" maxlength="500" placeholder="记录本次新增或调整的内容" /></label>
                <button class="admin-primary" type="button" :disabled="saving || !canPublishVersion" @click="publishVersion">发布新版本</button>
              </div>
              <p class="admin-note">版本会保存发布时的文档清单、处理状态和版本说明，便于追踪变更。文件原件和索引仍由 Coze 托管；这不是文件内容备份，删除远端文档后不能仅靠版本记录自动恢复。</p>
              <div class="admin-table-wrap knowledge-version-table">
                <table class="admin-table">
                  <thead><tr><th>版本</th><th>说明</th><th>文档数</th><th>发布时间</th><th>操作</th></tr></thead>
                  <tbody>
                    <template v-for="version in versions" :key="version.id">
                      <tr>
                        <td><strong>v{{ version.version_number }} · {{ version.title }}</strong></td>
                        <td>{{ version.description || '-' }}</td>
                        <td>{{ version.document_count }}</td>
                        <td>{{ formatDate(version.published_at) }}</td>
                        <td><button class="admin-text-button" type="button" @click="toggleVersion(version.id)">{{ expandedVersionId === Number(version.id) ? '收起清单' : '查看清单' }}</button></td>
                      </tr>
                      <tr v-if="expandedVersionId === Number(version.id)">
                        <td colspan="5">
                          <ul class="knowledge-version-docs">
                            <li v-for="document in version.documents || []" :key="document.document_id || document.id || document.name">
                              <strong>{{ document.name || document.document_name || '未命名文档' }}</strong>
                              <span>{{ documentStatus(document) }}</span>
                            </li>
                          </ul>
                        </td>
                      </tr>
                    </template>
                    <tr v-if="!versions.length"><td colspan="5" class="admin-empty">还没有发布版本</td></tr>
                  </tbody>
                </table>
              </div>
            </section>
            <p class="admin-note">注意：当前 OpenAI 兼容聊天链路不会自动检索 Coze 知识库。要让 Coze 智能体使用知识库，还需要在 Coze 智能体中关联对应数据集；本页面负责数据集和版本记录管理。</p>
          </template>
          <p v-else class="admin-empty">请先创建或选择一个知识库。</p>
        </section>
      </div>
    </template>
  </main>
</template>
