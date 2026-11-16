<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'

const props = defineProps({ adminMode: { type: Boolean, default: false } })
const canConfigurePermissions = ref(props.adminMode)
const listPath = computed(() => canConfigurePermissions.value ? '/knowledge/manage/bases' : '/knowledge/my-bases')
const bases = ref([])
const selectedBaseId = ref('')
const documents = ref([])
const versions = ref([])
const selectedDocument = ref(null)
const chunks = ref([])
const chunksLoading = ref(false)
const chunkSaving = ref(false)
const editingChunkId = ref('')
const editChunkContent = ref('')
const newChunkContent = ref('')
const loading = ref(false)
const saving = ref(false)
const uploading = ref(false)
const publishing = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const newName = ref('')
const newDescription = ref('')
const newAccessMode = ref('shared')
const newOwnerUsername = ref('')
const newMemberUsernames = ref('')
const editAccessMode = ref('private')
const editOwnerUsername = ref('')
const editMemberUsernames = ref('')
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
    const data = assertSuccess(await api.get(listPath.value))
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
  selectedDocument.value = null
  chunks.value = []
  editingChunkId.value = ''
  editChunkContent.value = ''
  newChunkContent.value = ''
  editName.value = base.name || ''
  editDescription.value = base.description || ''
  editEnabled.value = Boolean(base.enabled)
  editAccessMode.value = base.access_mode || 'private'
  editOwnerUsername.value = base.owner_username || ''
  editMemberUsernames.value = Array.isArray(base.member_usernames) ? base.member_usernames.join(', ') : ''
  const [documentData, versionData] = await Promise.all([
    api.get('/knowledge/manage/documents', { base_id: base.id }),
    api.get('/knowledge/manage/versions', { base_id: base.id }),
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

async function loadDocumentChunks(document) {
  selectedDocument.value = document
  chunks.value = []
  editingChunkId.value = ''
  editChunkContent.value = ''
  chunksLoading.value = true
  clearMessages()
  try {
    const data = assertSuccess(await api.get('/knowledge/manage/chunks', { document_id: document.id }))
    chunks.value = Array.isArray(data) ? data : []
  } catch (error) {
    errorMessage.value = error?.message || '读取文档分块失败'
  } finally {
    chunksLoading.value = false
  }
}

function closeChunkPanel() {
  selectedDocument.value = null
  chunks.value = []
  editingChunkId.value = ''
  editChunkContent.value = ''
  newChunkContent.value = ''
}

function beginEditChunk(chunk) {
  editingChunkId.value = String(chunk.id)
  editChunkContent.value = chunk.content || ''
}

function cancelEditChunk() {
  editingChunkId.value = ''
  editChunkContent.value = ''
}

async function saveChunk(chunk) {
  const content = editChunkContent.value.trim()
  if (!content) {
    errorMessage.value = '分块内容不能为空。'
    return
  }
  chunkSaving.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/knowledge/manage/chunks/update', {
      chunk_id: chunk.id,
      content,
    }))
    editingChunkId.value = ''
    editChunkContent.value = ''
    await loadDocumentChunks(selectedDocument.value)
    successMessage.value = '分块已保存，新的内容已立即用于知识库检索。'
  } catch (error) {
    errorMessage.value = error?.message || '保存分块失败'
  } finally {
    chunkSaving.value = false
  }
}

async function addChunk() {
  if (!selectedDocument.value || !newChunkContent.value.trim()) {
    errorMessage.value = '请先填写要添加的分块内容。'
    return
  }
  chunkSaving.value = true
  clearMessages()
  try {
    const document = selectedDocument.value
    assertSuccess(await api.post('/knowledge/manage/chunks/create', {
      document_id: document.id,
      content: newChunkContent.value.trim(),
    }))
    newChunkContent.value = ''
    await loadDocumentChunks(document)
    document.chunk_count = chunks.value.length
    successMessage.value = '新分块已添加，并已立即用于知识库检索。'
  } catch (error) {
    errorMessage.value = error?.message || '添加分块失败'
  } finally {
    chunkSaving.value = false
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
    const payload = { name: newName.value.trim(), description: newDescription.value.trim() }
    if (canConfigurePermissions.value) {
      payload.access_mode = newAccessMode.value
      payload.owner_username = newOwnerUsername.value.trim()
      payload.member_usernames = newMemberUsernames.value.trim()
    }
    const data = assertSuccess(await api.post('/knowledge/manage/bases/create', payload))
    newName.value = ''
    newDescription.value = ''
    newAccessMode.value = 'shared'
    newOwnerUsername.value = ''
    newMemberUsernames.value = ''
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
    const payload = {
      id: selectedBase.value.id,
      name: editName.value.trim(),
      description: editDescription.value.trim(),
      enabled: editEnabled.value,
    }
    if (canConfigurePermissions.value) {
      payload.access_mode = editAccessMode.value
      payload.owner_username = editOwnerUsername.value.trim()
      payload.member_usernames = editMemberUsernames.value.trim()
    }
    assertSuccess(await api.post('/knowledge/manage/bases/update', payload))
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
    assertSuccess(await api.post('/knowledge/manage/bases/delete', { id: selectedBase.value.id }))
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
    const data = assertSuccess(await api.post('/knowledge/manage/documents/upload', body))
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
  if (selectedDocument.value?.id === document.id) {
    selectedDocument.value = null
    chunks.value = []
  }
  saving.value = true
  clearMessages()
  try {
    assertSuccess(await api.post('/knowledge/manage/documents/delete', { id: document.id }))
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
    assertSuccess(await api.post('/knowledge/manage/versions/publish', {
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

function accessModeLabel(value) {
  return ({ shared: '公共知识库', assigned: '指定用户', private: '个人私有' })[value] || '个人私有'
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

onMounted(async () => {
  if (!canConfigurePermissions.value) {
    try {
      const profile = assertSuccess(await api.get('/auth/me'))
      canConfigurePermissions.value = Boolean(profile.is_admin)
    } catch {
      canConfigurePermissions.value = false
    }
  }
  await loadBases()
})
</script>

<template>
  <div class="knowledge-manager">
    <section class="admin-card">
      <div class="admin-card-heading">
        <div>
          <h2 class="knowledge-heading-with-icon"><img src="/zhishiku.png" alt="" aria-hidden="true" />{{ canConfigurePermissions ? '创建知识库' : '创建个人知识库' }}</h2>
          <p>{{ canConfigurePermissions ? '设置知识库归属与使用范围，授权用户只可检索，不能修改文档。' : '新建知识库默认仅自己和平台管理员可访问。' }}</p>
        </div>
      </div>
      <form class="knowledge-create-form" @submit.prevent="createBase">
        <label>知识库名称
          <input v-model="newName" maxlength="100" placeholder="例如：学校规章制度" required />
        </label>
        <label>知识库说明
          <input v-model="newDescription" maxlength="500" placeholder="简要说明知识库内容" />
        </label>
        <label v-if="canConfigurePermissions">访问权限
          <select v-model="newAccessMode">
            <option value="shared">公共：所有用户可用</option>
            <option value="assigned">指定用户可用</option>
            <option value="private">私有：仅归属人可用</option>
          </select>
        </label>
        <label v-if="canConfigurePermissions">归属账号
          <input v-model="newOwnerUsername" maxlength="100" placeholder="留空则归当前管理员" />
        </label>
        <label v-if="canConfigurePermissions && newAccessMode === 'assigned'">授权账号
          <textarea v-model="newMemberUsernames" rows="1" placeholder="多个账号用逗号分隔" required></textarea>
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
          <div><h2 class="knowledge-heading-with-icon"><img src="/zhishiku.png" alt="" aria-hidden="true" />知识库列表</h2><p>{{ bases.length }} 个知识库</p></div>
          <button class="admin-secondary" type="button" :disabled="saving" @click="loadBases">刷新</button>
        </div>
        <button v-for="base in bases" :key="base.id" type="button"
          class="knowledge-list-item" :class="{ active: base.id === selectedBaseId }"
          @click="selectBase(base.id)">
          <span class="knowledge-list-item-title"><img src="/zhishiku.png" alt="" aria-hidden="true" /><strong>{{ base.name }}</strong></span>
          <small>{{ base.document_count }} 份文档 · {{ accessModeLabel(base.access_mode) }} · {{ base.enabled ? '已启用' : '已停用' }}</small>
        </button>
        <p v-if="!bases.length" class="admin-empty">还没有知识库，请先创建。</p>
      </aside>

      <section class="knowledge-content admin-card">
        <template v-if="selectedBase">
          <div class="admin-card-heading">
            <div>
              <h2 class="knowledge-heading-with-icon"><img src="/zhishiku.png" alt="" aria-hidden="true" />{{ selectedBase.name }}</h2>
              <p>{{ selectedBase.description || '尚未填写知识库说明' }}</p>
              <p class="knowledge-owner-line">归属：{{ selectedBase.owner_username || '未知' }} · {{ accessModeLabel(selectedBase.access_mode) }}<template v-if="canConfigurePermissions && selectedBase.access_mode === 'assigned'"> · 授权：{{ (selectedBase.member_usernames || []).join('、') || '暂无' }}</template></p>
            </div>
            <span class="admin-mode">{{ selectedBase.enabled ? '对话可用' : '已停用' }}</span>
          </div>

          <div class="knowledge-base-settings">
            <label>知识库名称<input v-model="editName" maxlength="100" /></label>
            <label>知识库说明<input v-model="editDescription" maxlength="500" /></label>
            <label class="knowledge-enabled-option"><input v-model="editEnabled" type="checkbox" /> 允许在对话中使用</label>
            <label v-if="canConfigurePermissions">访问权限
              <select v-model="editAccessMode">
                <option value="shared">公共：所有用户可用</option>
                <option value="assigned">指定用户可用</option>
                <option value="private">私有：仅归属人可用</option>
              </select>
            </label>
            <label v-if="canConfigurePermissions">归属账号
              <input v-model="editOwnerUsername" maxlength="100" placeholder="填写登录账号" required />
            </label>
            <label v-if="canConfigurePermissions && editAccessMode === 'assigned'">授权账号
              <textarea v-model="editMemberUsernames" rows="2" placeholder="多个账号用逗号分隔" required></textarea>
            </label>
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
              <thead><tr><th>文档名称</th><th>大小</th><th>文本分块</th><th>上传时间</th><th>操作</th></tr></thead>
              <tbody>
                <tr v-for="document in documents" :key="document.id">
                  <td>{{ document.filename }}</td>
                  <td>{{ formatBytes(document.size_bytes) }}</td>
                  <td>{{ document.chunk_count }}</td>
                  <td>{{ formatDate(document.created_at) }}</td>
                  <td class="knowledge-document-actions">
                    <button class="admin-text-button" type="button" :disabled="chunksLoading" @click="loadDocumentChunks(document)">查看分块</button>
                    <button class="admin-text-button danger" type="button" :disabled="saving" @click="deleteDocument(document)">删除</button>
                  </td>
                </tr>
                <tr v-if="!documents.length"><td colspan="5" class="admin-empty">暂无文档，请上传需要检索的资料。</td></tr>
              </tbody>
            </table>
          </div>

          <section v-if="selectedDocument" class="knowledge-chunk-panel">
            <div class="knowledge-chunk-heading">
              <div>
                <h3>文档分块管理</h3>
                <p>{{ selectedDocument.filename }} · {{ chunks.length }} 个分块</p>
              </div>
              <button class="admin-secondary" type="button" :disabled="chunkSaving" @click="closeChunkPanel">收起</button>
            </div>

            <div class="knowledge-add-chunk">
              <div class="knowledge-add-chunk-heading">
                <strong>添加新分块</strong>
                <span>最多 12000 字符</span>
              </div>
              <textarea v-model="newChunkContent" rows="4" maxlength="12000" placeholder="输入需要补充到该文档的新知识内容…" :disabled="chunkSaving"></textarea>
              <div class="knowledge-chunk-actions">
                <span>{{ newChunkContent.length }} / 12000</span>
                <button class="admin-primary" type="button" :disabled="chunkSaving || !newChunkContent.trim()" @click="addChunk">{{ chunkSaving ? '保存中…' : '添加分块' }}</button>
              </div>
            </div>

            <div v-if="chunksLoading" class="admin-empty">正在读取分块内容…</div>
            <div v-else-if="!chunks.length" class="admin-empty">该文档还没有分块，可以在上方手动添加。</div>
            <div v-else class="knowledge-chunk-list">
              <article v-for="(chunk, index) in chunks" :key="chunk.id" class="knowledge-chunk-item">
                <div class="knowledge-chunk-item-heading">
                  <div>
                    <strong>分块 {{ Number(chunk.chunk_index) + 1 }}</strong>
                    <span>{{ chunk.character_count ?? (chunk.content || '').length }} 字</span>
                  </div>
                  <button v-if="editingChunkId !== String(chunk.id)" class="admin-text-button" type="button" :disabled="chunkSaving" @click="beginEditChunk(chunk)">编辑内容</button>
                </div>
                <template v-if="editingChunkId === String(chunk.id)">
                  <textarea v-model="editChunkContent" class="knowledge-chunk-editor" rows="7" maxlength="12000" :disabled="chunkSaving"></textarea>
                  <div class="knowledge-chunk-actions">
                    <span>{{ editChunkContent.length }} / 12000</span>
                    <div>
                      <button class="admin-secondary" type="button" :disabled="chunkSaving" @click="cancelEditChunk">取消</button>
                      <button class="admin-primary" type="button" :disabled="chunkSaving || !editChunkContent.trim()" @click="saveChunk(chunk)">{{ chunkSaving ? '保存中…' : '保存修改' }}</button>
                    </div>
                  </div>
                </template>
                <pre v-else class="knowledge-chunk-content">{{ chunk.content }}</pre>
              </article>
            </div>
          </section>

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
          <div class="knowledge-empty-icon"><img src="/zhishiku.png" alt="" aria-hidden="true" /></div>
          <h2>{{ canConfigurePermissions ? '选择一个知识库' : '创建你的第一个知识库' }}</h2>
          <p>{{ canConfigurePermissions ? '选择左侧知识库查看文档，并在上方设置使用权限。' : '上传个人资料后，只有你本人和平台管理员可以查看与检索。' }}</p>
        </div>
      </section>
    </div>
  </div>
</template>
