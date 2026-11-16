<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'
import { clearAuth } from '../auth'
import { copy } from '../copy'

const router = useRouter()
const username = ref(localStorage.getItem('username') || '用户')
const isAdmin = ref(false)
const models = ref([])
const knowledgeBases = ref([])
const knowledgeEnabled = ref(false)
const selectedKnowledgeBaseId = ref('')
const authMode = ref('local')
const selectedBot = ref(null)
const conversationId = ref('')
const messages = ref([])
const conversations = ref([])
const draft = ref('')
const attachment = ref(null)
const isSending = ref(false)
const isLoading = ref(false)
const historyLoading = ref(false)
const historyError = ref('')
const pageError = ref('')
const showNoProviderModal = ref(false)
const showHistoryDrawer = ref(false)
const messageList = ref(null)
let disposed = false

const usernameInitial = computed(() => (username.value || 'U').trim().slice(0, 1).toUpperCase())
const canSend = computed(() => !isSending.value && Boolean(draft.value.trim()))

function assertSuccess(result) {
  if (result?.error) throw new Error(result.msg || copy.requestFailed)
  const code = Number(result?.code)
  if (Number.isFinite(code) && code !== 0 && code !== 200) {
    throw new Error(result?.msg || copy.requestFailed)
  }
  return result?.data ?? {}
}

function scrollToBottom() {
  nextTick(() => {
    if (messageList.value) messageList.value.scrollTop = messageList.value.scrollHeight
  })
}

async function loadModels() {
  const data = assertSuccess(await api.get('/agents/models'))
  models.value = Array.isArray(data.models) ? data.models : []
  const defaultModel = data.default_model
  const preferred = models.value.find((item) => item.name === defaultModel) || models.value[0] || null
  if (!models.value.some((item) => item.bot_id === selectedBot.value?.bot_id)) {
    selectedBot.value = preferred
  }
}

async function loadKnowledgeBases() {
  const data = assertSuccess(await api.get('/knowledge/bases'))
  knowledgeBases.value = Array.isArray(data) ? data : []
  if (!knowledgeBases.value.some((item) => item.id === selectedKnowledgeBaseId.value)) {
    selectedKnowledgeBaseId.value = knowledgeBases.value[0]?.id || ''
  }
  if (!knowledgeBases.value.length) knowledgeEnabled.value = false
}

async function createConversation() {
  const payload = selectedBot.value?.bot_id ? { bot_id: selectedBot.value.bot_id } : {}
  const result = await api.post('/agents/createConversation', payload)
  const data = assertSuccess(result)
  if (!data.id) throw new Error(copy.noConversation)
  conversationId.value = data.id
  return data.id
}

async function loadConversationHistory() {
  historyLoading.value = true
  historyError.value = ''
  try {
    const data = assertSuccess(await api.get('/agents/listConversations', { limit: 100 }))
    conversations.value = Array.isArray(data) ? data : []
  } catch (error) {
    historyError.value = error?.message || '读取历史会话失败'
  } finally {
    historyLoading.value = false
  }
}

async function initialize() {
  isLoading.value = true
  pageError.value = ''
  try {
    const profile = assertSuccess(await api.get('/auth/me'))
    username.value = profile.display_name || profile.account || '用户'
    isAdmin.value = Boolean(profile.is_admin)
    localStorage.setItem('username', username.value)
    const auth = assertSuccess(await api.get('/auth/mode', undefined, { auth: false }))
    authMode.value = auth.mode || 'local'
    await loadModels()
    await loadKnowledgeBases()
    await loadConversationHistory()
    if (selectedBot.value) await createConversation()
  } catch (error) {
    pageError.value = error?.message || copy.unavailable
  } finally {
    isLoading.value = false
  }
}

function normalizeContent(value) {
  if (typeof value === 'string') return value
  if (Array.isArray(value)) {
    return value.map((part) => part?.text || part?.content || '').filter(Boolean).join('\n')
  }
  return value == null ? '' : JSON.stringify(value)
}

function formatDate(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function wait(milliseconds) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}

async function pollForChat(chatId, assistantIndex) {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    if (disposed) return
    await wait(attempt === 0 ? 400 : 1000)
    const detailResult = await api.get('/agents/getChatDetail', {
      conversation_id: conversationId.value,
      chat_id: chatId,
    })
    const detail = assertSuccess(detailResult)
    if (detail.status === 'failed') throw new Error(detail.last_error || copy.requestFailed)
    if (detail.status === 'completed' || detail.status === 'success') {
      const messageResult = await api.get('/agents/getChatMessage', {
        conversation_id: conversationId.value,
        chat_id: chatId,
      })
      const data = assertSuccess(messageResult)
      const list = Array.isArray(data) ? data : (Array.isArray(data.messages) ? data.messages : [])
      const assistant = [...list].reverse().find((item) => item?.role === 'assistant') || list[1]
      if (!assistant) throw new Error(copy.requestFailed)
      messages.value[assistantIndex] = {
        id: assistant.id || chatId,
        role: 'assistant',
        content: normalizeContent(assistant.content ?? assistant.text),
        reasoning_content: normalizeContent(assistant.reasoning_content),
        pending: false,
      }
      scrollToBottom()
      return
    }
  }
  throw new Error(copy.requestFailed)
}

function isImageFile(file) {
  return /\.(png|jpe?g)$/i.test(file?.name || '')
}

function selectModel(botID) {
  selectedBot.value = models.value.find((item) => item.bot_id === botID) || null
  pageError.value = ''
}

async function openConversation(item) {
  if (isSending.value) return
  historyError.value = ''
  try {
    const data = assertSuccess(await api.post('/agents/getMessageList', {
      conversation_id: item.id,
      order: 'asc',
      limit: 100,
    }))
    const list = Array.isArray(data) ? data : []
    messages.value = list
      .filter((message) => message.role === 'user' || message.role === 'assistant')
      .map((message) => ({
        id: message.id,
        role: message.role,
        content: normalizeContent(message.content),
        reasoning_content: normalizeContent(message.reasoning_content),
        pending: false,
      }))
    conversationId.value = item.id
    const matchingModel = models.value.find((model) => model.bot_id === item.bot_id)
    if (matchingModel) selectedBot.value = matchingModel
    showHistoryDrawer.value = false
    pageError.value = ''
    scrollToBottom()
  } catch (error) {
    historyError.value = error?.message || '打开历史会话失败'
  }
}

async function deleteConversation(item) {
  if (!window.confirm('确定删除这条历史对话吗？此操作不可撤销。')) return
  try {
    assertSuccess(await api.post('/agents/deleteConversation', { conversation_id: item.id }))
    conversations.value = conversations.value.filter((entry) => entry.id !== item.id)
    if (conversationId.value === item.id) {
      conversationId.value = ''
      messages.value = []
      if (selectedBot.value) await createConversation()
    }
  } catch (error) {
    historyError.value = error?.message || '删除历史对话失败'
  }
}

async function sendMessage(text = draft.value, options = {}) {
  const content = String(text || '').trim()
  if (!content || isSending.value) return

  isSending.value = true
  pageError.value = ''
  try {
    // Refresh on send as well as on page load, so a provider activated in
    // another tab becomes available without a browser or service restart.
    await loadModels()
  } catch (error) {
    pageError.value = error?.message || copy.requestFailed
    isSending.value = false
    return
  }
  if (!selectedBot.value?.bot_id) {
    showNoProviderModal.value = true
    isSending.value = false
    return
  }
  showNoProviderModal.value = false
  const selectedFile = options.regenerate ? null : attachment.value
  if (!conversationId.value) {
    try {
      await createConversation()
    } catch (error) {
      pageError.value = error?.message || copy.noConversation
      isSending.value = false
      return
    }
  }

  if (!options.regenerate) {
    messages.value.push({ id: 'user-' + Date.now(), role: 'user', content })
  }
  const assistantIndex = messages.value.push({
    id: 'assistant-' + Date.now(),
    role: 'assistant',
    content: '',
    reasoning_content: '',
    pending: true,
  }) - 1
  if (!options.regenerate) {
    draft.value = ''
    attachment.value = null
  }
  scrollToBottom()

  try {
    const form = new FormData()
    form.append('conversation_id', conversationId.value)
    form.append('bot_id', selectedBot.value.bot_id)
    form.append('stream', '2')
    form.append('content', content)
    form.append('knowledge_base_id', knowledgeEnabled.value ? selectedKnowledgeBaseId.value : '')
    form.append('up_type', selectedFile ? (isImageFile(selectedFile) ? 'image' : 'file') : '')
    form.append('url', '')
    if (selectedFile) form.append('file', selectedFile)

    const result = await api.post('/agents/chat', form)
    const data = assertSuccess(result)
    const chatId = data.id || data.chat_id
    if (!chatId) throw new Error(result?.msg || copy.requestFailed)
    await pollForChat(chatId, assistantIndex)
    await loadConversationHistory()
  } catch (error) {
    if (messages.value[assistantIndex]) {
      messages.value[assistantIndex] = {
        ...messages.value[assistantIndex],
        content: error?.message || copy.requestFailed,
        pending: false,
        failed: true,
      }
    }
    const message = error?.message || copy.requestFailed
    if (message.includes('当前没有配置模型渠道')) {
      messages.value.splice(options.regenerate ? assistantIndex : assistantIndex - 1, options.regenerate ? 1 : 2)
      if (!options.regenerate) {
        draft.value = content
        attachment.value = selectedFile
      }
      showNoProviderModal.value = true
      pageError.value = ''
    } else {
      pageError.value = message
    }
    scrollToBottom()
  } finally {
    isSending.value = false
  }
}

function handleFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  const extension = file.name.split('.').pop().toLowerCase()
  const allowed = ['txt', 'jpg', 'jpeg', 'png']
  if (!allowed.includes(extension)) {
    pageError.value = copy.uploadUnsupported
    return
  }
  pageError.value = ''
  attachment.value = file
}

function removeAttachment() {
  attachment.value = null
}

function handleComposerKeydown(event) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    sendMessage()
  }
}

async function copyMessage(content) {
  if (!content) return
  try {
    await navigator.clipboard.writeText(content)
  } catch {
    const input = document.createElement('textarea')
    input.value = content
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    input.remove()
  }
}

function regenerateMessage(index) {
  const previousUser = [...messages.value.slice(0, index)].reverse().find((item) => item.role === 'user')
  if (previousUser) sendMessage(previousUser.content, { regenerate: true })
}

async function startNewConversation() {
  if (isSending.value) return
  messages.value = []
  draft.value = ''
  attachment.value = null
  pageError.value = ''
  showHistoryDrawer.value = false
  conversationId.value = ''
  try {
    if (selectedBot.value) await createConversation()
  } catch (error) {
    pageError.value = error?.message || copy.noConversation
  }
}

async function logout() {
  try {
    await api.post('/auth/logout', {}, { auth: false })
  } catch {
    // Clear local credentials even if the server session has already expired.
  }
  clearAuth()
  if (authMode.value === 'sso') {
    window.location.assign('/auth/sso/logout')
    return
  }
  await router.replace('/login')
}

onMounted(initialize)
onBeforeUnmount(() => {
  disposed = true
})
</script>

<template>
  <main class="chat-page">
    <header class="chat-header">
      <button class="icon-button" type="button" :aria-label="copy.history" @click="showHistoryDrawer = true">&#9776;</button>
      <div class="chat-header-title">
        <BrandMark class="header-mark" />
        <span>{{ copy.appName }}</span>
      </div>
      <button class="icon-button new-chat-button" type="button" :aria-label="copy.startNewChat" :disabled="isSending" @click="startNewConversation">+</button>
    </header>

    <section ref="messageList" class="message-list" aria-live="polite">
      <div v-if="!messages.length && !isLoading" class="welcome-panel">
        <BrandMark class="welcome-mark" />
        <h1>{{ copy.emptyTitle }}</h1>
        <p>{{ copy.emptyDescription }}</p>
      </div>
      <div v-if="isLoading" class="loading-state">{{ copy.loading }}</div>

      <article v-for="(message, index) in messages" :key="message.id || index" class="message-row" :class="message.role === 'user' ? 'message-row-user' : 'message-row-assistant'">
        <template v-if="message.role === 'user'">
          <div class="user-bubble">{{ message.content }}</div>
        </template>
        <template v-else>
          <BrandMark class="assistant-avatar" />
          <div class="assistant-card">
            <div class="assistant-status">
              <span class="status-dot" :class="{ busy: message.pending }"></span>
              {{ message.pending ? copy.thinking : copy.thinkingDone }}
            </div>
            <details v-if="message.reasoning_content" class="reasoning-block">
              <summary>{{ copy.reasoning }}</summary>
              <p>{{ message.reasoning_content }}</p>
            </details>
            <div class="assistant-content" :class="{ 'assistant-content-failed': message.failed }">{{ message.pending ? '' : message.content }}</div>
            <div v-if="!message.pending && message.content" class="message-actions">
              <button type="button" @click="regenerateMessage(index)">{{ copy.regenerate }}</button>
              <button type="button" @click="copyMessage(message.content)">{{ copy.copy }}</button>
            </div>
          </div>
        </template>
      </article>
      <div v-if="pageError" class="page-error" role="status">{{ pageError }}</div>
    </section>

    <footer class="chat-footer">
      <div v-if="models.length || knowledgeBases.length" class="footer-toolbar">
        <label v-if="models.length" class="model-select-label">
          <span>模型</span>
          <select :value="selectedBot?.bot_id || ''" :disabled="!models.length || isSending" @change="selectModel($event.target.value)">
            <option v-for="model in models" :key="model.bot_id" :value="model.bot_id">{{ model.name }}{{ model.default ? '（默认）' : '' }}</option>
          </select>
        </label>
        <span v-if="selectedBot" class="model-current-hint">当前对话使用 {{ selectedBot.name }}</span>
        <label class="knowledge-toggle">
          <input v-model="knowledgeEnabled" type="checkbox" :disabled="!knowledgeBases.length || isSending" />
          <img class="knowledge-toggle-icon" src="/zhishiku.png" alt="" aria-hidden="true" />
          <span>使用知识库</span>
        </label>
        <select v-if="knowledgeBases.length" v-model="selectedKnowledgeBaseId" class="knowledge-base-select" :disabled="!knowledgeEnabled || isSending">
          <option v-for="base in knowledgeBases" :key="base.id" :value="base.id">{{ base.name }}</option>
        </select>
      </div>
      <div class="composer-shell">
        <div v-if="attachment" class="attachment-chip">
          <span class="attachment-icon">&#128196;</span>
          <span class="attachment-name">{{ attachment.name }}</span>
          <button type="button" :aria-label="copy.removeFile" @click="removeAttachment">&times;</button>
        </div>
        <textarea
          v-model="draft"
          class="composer-input"
          rows="1"
          :placeholder="copy.sendPlaceholder"
          :disabled="isSending"
          @keydown="handleComposerKeydown"
        ></textarea>
        <div class="composer-actions">
          <label class="attach-button" :title="copy.attach">
            <input type="file" accept=".txt,.jpg,.jpeg,.png" :disabled="isSending" @change="handleFileChange" />
            <span aria-hidden="true">+</span>
          </label>
          <button class="send-button" type="button" :disabled="!canSend" :aria-label="copy.send" @click="sendMessage()">&#8593;</button>
        </div>
      </div>
    </footer>

    <div v-if="showNoProviderModal" class="provider-required-backdrop" @click.self="showNoProviderModal = false">
      <section class="provider-required-dialog" role="dialog" aria-modal="true" aria-labelledby="provider-required-title">
        <div class="provider-required-icon">!</div>
        <h2 id="provider-required-title">当前没有配置模型渠道</h2>
        <p>请联系管理员配置并启用 AI 模型供应商后，再开始对话。</p>
        <div class="provider-required-actions">
          <button class="provider-required-dismiss" type="button" @click="showNoProviderModal = false">我知道了</button>
          <button v-if="isAdmin" class="provider-required-admin" type="button" @click="showNoProviderModal = false; router.push('/admin')">前往配置</button>
        </div>
      </section>
    </div>

    <div v-if="showHistoryDrawer" class="drawer-backdrop" @click.self="showHistoryDrawer = false">
      <aside class="history-drawer" :aria-label="copy.history">
        <button class="start-chat-button" type="button" :disabled="isSending" @click="startNewConversation">+ {{ copy.startNewChat }}</button>
        <div class="history-list">
          <div v-if="historyLoading" class="history-status">{{ copy.loading }}</div>
          <div v-else-if="historyError" class="history-status history-error">{{ historyError }}</div>
          <div v-else-if="!conversations.length" class="history-empty-state">
            <span class="history-empty-icon">&#128196;</span>
            <strong>{{ copy.noHistory }}</strong>
            <small>{{ copy.historyHint }}</small>
          </div>
          <div v-else v-for="item in conversations" :key="item.id" class="history-item" :class="{ active: item.id === conversationId }">
            <button class="history-item-open" type="button" @click="openConversation(item)">
              <strong>{{ item.title || copy.newChat }}</strong>
              <small>{{ formatDate(item.updated_at) }}</small>
            </button>
            <button class="history-item-delete" type="button" :aria-label="'删除 ' + (item.title || copy.newChat)" @click="deleteConversation(item)">×</button>
          </div>
        </div>
        <footer class="drawer-user">
          <span class="user-avatar">{{ usernameInitial }}</span>
          <strong>{{ username }}</strong>
          <button type="button" @click="router.push('/my-knowledge')">我的知识库</button>
          <button v-if="isAdmin" type="button" @click="router.push('/admin')">平台管理</button>
          <button type="button" @click="logout">{{ copy.logout }}</button>
        </footer>
      </aside>
    </div>
  </main>
</template>
