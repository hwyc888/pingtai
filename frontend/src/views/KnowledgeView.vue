<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const loading = ref(true)
const saving = ref(false)
const available = ref(false)
const workspaceId = ref('')
const datasets = ref([])
const selectedDataset = ref(null)
const documents = ref([])
const errorMessage = ref('')
const successMessage = ref('')
const datasetName = ref('')
const datasetDescription = ref('')
const uploadFiles = ref([])
const selectedFile = ref(null)
const filePage = ref(1)
const fileTotal = ref(0)

const selectedTitle = computed(() => selectedDataset.value?.name || '选择知识库')

function unwrap(result) {
  if (result?.error || (Number.isFinite(Number(result?.code)) && ![0, 200].includes(Number(result.code)))) {
    throw new Error(result?.msg || '请求失败')
  }
  return result?.data ?? {}
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
    selectedDataset.value = datasets.value.find((item) => item.dataset_id === selectedDataset.value.dataset_id || item.id === selectedDataset.value.id) || null
  }
  if (!selectedDataset.value && datasets.value.length) selectDataset(datasets.value[0])
}

async function createDataset() {
  if (!datasetName.value.trim()) {
    errorMessage.value = '请输入知识库名称。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  try {
    const result = unwrap(await api.post('/datasets/createKnowledge', {
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
  filePage.value = 1
  await loadDocuments()
}

function datasetID(item = selectedDataset.value) {
  return item?.dataset_id || item?.id || ''
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

function onFilesChanged(event) {
  uploadFiles.value = Array.from(event.target.files || [])
}

async function uploadDocuments() {
  if (!datasetID() || !uploadFiles.value.length) {
    errorMessage.value = '请选择知识库和要上传的文件。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  try {
    const form = new FormData()
    form.append('dataset_id', datasetID())
    form.append('format_type', '0')
    form.append('source', 'file')
    for (const file of uploadFiles.value) form.append('files', file)
    unwrap(await api.post('/datasets/createKnowledgeFile', form))
    successMessage.value = '文档上传请求已提交，正在等待知识库处理。'
    uploadFiles.value = []
    const input = document.getElementById('knowledge-files')
    if (input) input.value = ''
    await loadDocuments()
  } catch (error) {
    errorMessage.value = error?.message || '上传知识库文档失败'
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
    successMessage.value = '文档删除请求已提交。'
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
    successMessage.value = '知识库删除请求已提交。'
    await loadDatasets()
  } catch (error) {
    errorMessage.value = error?.message || '删除知识库失败'
  } finally {
    saving.value = false
  }
}

async function refresh() {
  if (selectedDataset.value) await loadDocuments()
  else await loadDatasets()
}

onMounted(load)
</script>

<template>
  <main class="knowledge-page">
    <header class="knowledge-header">
      <div>
        <p class="admin-eyebrow">PINGTAI KNOWLEDGE</p>
        <h1>知识库管理</h1>
        <p class="admin-subtitle">管理知识库、上传资料并查看文档处理状态。</p>
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
      <div class="knowledge-empty-icon">⌘</div>
      <h2>知识库服务尚未连接</h2>
      <p>当前聊天使用 OpenAI 兼容 API。知识库管理仍保留原有 Coze 知识库接口，因此需要配置 Coze Access Token，或 Coze OAuth 应用凭据。</p>
      <p>配置完成后即可恢复知识库和文档管理。不会把普通聊天 API Key 当作知识库凭据。</p>
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
              <span>{{ uploadFiles.length ? uploadFiles.map((file) => file.name).join('、') : '支持 TXT、PDF、DOC、DOCX' }}</span>
              <button class="admin-primary" type="button" :disabled="saving || !uploadFiles.length" @click="uploadDocuments">上传文档</button>
            </div>
            <div class="admin-table-wrap">
              <table class="admin-table knowledge-table">
                <thead><tr><th>文档名称</th><th>状态</th><th>更新时间</th><th>操作</th></tr></thead>
                <tbody>
                  <tr v-for="item in documents" :key="item.document_id || item.id">
                    <td>{{ item.name || item.document_name || item.display_name || '未命名文档' }}</td>
                    <td>{{ item.status || item.process_status || '未知' }}</td>
                    <td>{{ item.update_time || item.updated_at || item.create_time || '-' }}</td>
                    <td><button class="admin-text-button danger" type="button" @click="deleteDocument(item)">删除</button></td>
                  </tr>
                  <tr v-if="!documents.length"><td colspan="4" class="admin-empty">该知识库暂无文档</td></tr>
                </tbody>
              </table>
            </div>
            <p class="admin-note">文档上传和删除通过 Coze 知识库服务处理；OpenAI 兼容聊天供应商不会自动获得这些文档的检索能力。</p>
          </template>
          <p v-else class="admin-empty">请先创建或选择一个知识库。</p>
        </section>
      </div>
    </template>
  </main>
</template>
