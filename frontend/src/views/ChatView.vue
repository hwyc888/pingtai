<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'
import { clearAuth } from '../auth'
import { copy } from '../copy'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import 'highlight.js/styles/github-dark.css'

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
let activeRequestController = null
let stopRequested = false

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

async function readEventStream(response, onEvent) {
  if (!response.body) throw new Error('当前浏览器不支持流式响应')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  async function dispatch(frame) {
    let event = 'message'
    const data = []
    for (const rawLine of frame.split('\n')) {
      const line = rawLine.replace(/\r$/, '')
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
    }
    if (data.length) await onEvent(event, data.join('\n'))
  }
  try {
    while (true) {
      const result = await reader.read()
      buffer += decoder.decode(result.value || new Uint8Array(), { stream: !result.done })
      buffer = buffer.replace(/\r\n/g, '\n')
      let boundary = buffer.indexOf('\n\n')
      while (boundary >= 0) {
        const frame = buffer.slice(0, boundary)
        buffer = buffer.slice(boundary + 2)
        await dispatch(frame)
        boundary = buffer.indexOf('\n\n')
      }
      if (result.done) break
    }
    if (buffer.trim()) await dispatch(buffer)
  } catch (error) {
    await reader.cancel().catch(() => {})
    throw error
  } finally {
    reader.releaseLock()
  }
}

function parseEventPayload(raw) {
  try { return JSON.parse(raw) } catch { return raw }
}

function appendAssistantContent(index, value) {
  if (typeof value !== 'string' || !value) return
  const message = messages.value[index]
  if (!message) return
  message.content += value
  scrollToBottom()
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[character])
}

function renderMarkdown(content) {
  const renderer = new marked.Renderer()
  renderer.code = ({ text, lang }) => {
    const language = String(lang || 'text').split(/\s+/)[0].toLowerCase()
    const safeLanguage = escapeHTML(language || 'text')
    let highlighted = escapeHTML(text)
    try {
      highlighted = language && hljs.getLanguage(language)
        ? hljs.highlight(text, { language }).value
        : hljs.highlightAuto(text).value
    } catch { /* Keep the escaped code if highlighting is unavailable. */ }
    return '<div class="markdown-code-block">' +
      '<div class="markdown-code-toolbar"><span>' + safeLanguage + '</span>' +
      '<div class="markdown-code-actions">' +
      '<button type="button" data-code-action="copy">复制代码</button>' +
      '<button type="button" data-code-action="save">保存代码</button>' +
      '</div></div><pre><code class="hljs language-' + safeLanguage + '">' + highlighted + '</code></pre></div>\n'
  }
  const html = marked.parse(String(content || ''), { renderer, gfm: true, breaks: true })
  return DOMPurify.sanitize(html, { ADD_ATTR: ['data-code-action'] })
}

async function copyText(value) {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
  } catch {
    const input = document.createElement('textarea')
    input.value = value
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    input.remove()
  }
}

function downloadText(value, filename, mimeType = 'text/plain;charset=utf-8') {
  const blob = new Blob([value], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function saveAssistantMarkdown(message, index) {
  if (message?.content) downloadText(message.content, '超凡AI回答-' + (index + 1) + '.md', 'text/markdown;charset=utf-8')
}

function saveAssistantText(message, index) {
  if (message?.content) downloadText(message.content, '超凡AI回答-' + (index + 1) + '.txt')
}

function handleAssistantAction(event, message, index) {
  const button = event.target.closest('[data-code-action]')
  if (!button) return
  const block = button.closest('.markdown-code-block')
  const code = block?.querySelector('pre code')?.textContent || ''
  if (!code) return
  if (button.dataset.codeAction === 'copy') {
    copyText(code)
    return
  }
  const match = block?.querySelector('code')?.className.match(/language-([a-z0-9+#.-]+)/i)
  const language = match?.[1]?.toLowerCase() || 'txt'
  const extensions = { javascript: 'js', typescript: 'ts', python: 'py', golang: 'go', go: 'go', bash: 'sh', shell: 'sh', json: 'json', html: 'html', css: 'css', sql: 'sql', java: 'java', rust: 'rs', csharp: 'cs', markdown: 'md' }
  downloadText(code, '超凡AI代码-' + (index + 1) + '.' + (extensions[language] || language || 'txt'))
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
  try { await loadModels() } catch (error) {
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
    try { await createConversation() } catch (error) {
      pageError.value = error?.message || copy.noConversation
      isSending.value = false
      return
    }
  }
  if (!options.regenerate) messages.value.push({ id: 'user-' + Date.now(), role: 'user', content })
  const assistantIndex = messages.value.push({
    id: 'assistant-' + Date.now(), role: 'assistant', content: '', reasoning_content: '',
    pending: true, stopped: false,
  }) - 1
  if (!options.regenerate) { draft.value = ''; attachment.value = null }
  scrollToBottom()
  const controller = new AbortController()
  activeRequestController = controller
  stopRequested = false
  let chatId = ''
  let cozeReceivedDelta = false
  try {
    const form = new FormData()
    form.append('conversation_id', conversationId.value)
    form.append('bot_id', selectedBot.value.bot_id)
    form.append('stream', '1')
    form.append('content', content)
    form.append('knowledge_base_id', knowledgeEnabled.value ? selectedKnowledgeBaseId.value : '')
    form.append('up_type', selectedFile ? (isImageFile(selectedFile) ? 'image' : 'file') : '')
    form.append('url', '')
    if (selectedFile) form.append('file', selectedFile)
    const response = await api.stream('/agents/chat', form, { signal: controller.signal, headers: { Accept: 'text/event-stream' } })
    const contentType = response.headers.get('content-type') || ''
    if (!contentType.includes('text/event-stream')) {
      const payload = await response.json().catch(() => ({}))
      throw new Error(payload?.msg || payload?.error?.message || '模型没有返回流式响应')
    }
    await readEventStream(response, async (event, raw) => {
      const payload = parseEventPayload(raw)
      const data = payload && typeof payload === 'object' ? payload : {}
      if (event === 'start' || event === 'conversation.chat.created') {
        chatId = data.id || data.chat_id || chatId
        if (chatId) messages.value[assistantIndex].id = chatId
      } else if (event === 'delta') {
        appendAssistantContent(assistantIndex, data.content)
      } else if (event === 'reasoning') {
        if (typeof data.content === 'string') { messages.value[assistantIndex].reasoning_content += data.content; scrollToBottom() }
      } else if (event === 'done') {
        chatId = data.id || chatId
        if (data.status === 'stopped') messages.value[assistantIndex].stopped = true
        if (data.status === 'failed') messages.value[assistantIndex].failed = true
      } else if (event === 'error') {
        throw new Error(data.message || (typeof payload === 'string' ? payload : '模型生成失败'))
      } else if (event === 'conversation.message.delta') {
        const isAnswer = data.type === 'answer' || (!data.type && data.role === 'assistant')
        if (isAnswer && typeof data.content === 'string') { cozeReceivedDelta = true; appendAssistantContent(assistantIndex, data.content) }
      } else if (event === 'conversation.message.completed') {
        if (!cozeReceivedDelta && data.type === 'answer' && typeof data.content === 'string') {
          messages.value[assistantIndex].content = data.content
          scrollToBottom()
        }
      } else if (event === 'conversation.chat.failed') {
        throw new Error(data.last_error || data.error_message || data.message || '模型生成失败')
      }
    })
    if (!stopRequested && !disposed) {
      messages.value[assistantIndex].pending = false
      await loadConversationHistory()
    }
  } catch (error) {
    const assistant = messages.value[assistantIndex]
    if (assistant) {
      assistant.pending = false
      if (stopRequested || error?.name === 'AbortError') assistant.stopped = true
      else { assistant.failed = true; if (!assistant.content) assistant.content = error?.message || copy.requestFailed }
    }
    if (stopRequested || error?.name === 'AbortError') {
      if (!disposed) await loadConversationHistory()
    } else {
      const message = error?.message || copy.requestFailed
      if (message.includes('当前没有配置模型渠道')) {
        messages.value.splice(options.regenerate ? assistantIndex : assistantIndex - 1, options.regenerate ? 1 : 2)
        if (!options.regenerate) { draft.value = content; attachment.value = selectedFile }
        showNoProviderModal.value = true
        pageError.value = ''
      } else pageError.value = message
    }
    scrollToBottom()
  } finally {
    if (activeRequestController === controller) activeRequestController = null
    isSending.value = false
    stopRequested = false
  }
}

function stopGeneration() {
  if (!activeRequestController || !isSending.value) return
  stopRequested = true
  activeRequestController.abort()
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

async function copyMessage(content) { await copyText(content) }

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
  if (activeRequestController) {
    stopRequested = true
    activeRequestController.abort()
  }
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
              <span class="status-dot" :class="{ busy: message.pending, stopped: message.stopped, failed: message.failed }"></span>
              {{ message.pending ? '正在生成' : message.stopped ? '已停止生成' : message.failed ? '生成未完成' : copy.thinkingDone }}
            </div>
            <details v-if="message.reasoning_content" class="reasoning-block">
              <summary>{{ copy.reasoning }}</summary>
              <p>{{ message.reasoning_content }}</p>
            </details>
            <div class="assistant-content" :class="{ 'assistant-content-failed': message.failed }" @click="handleAssistantAction($event, message, index)" v-html="message.pending && !message.content ? '' : renderMarkdown(message.content)"></div>
            <div v-if="message.content" class="message-actions">
              <button type="button" @click="copyMessage(message.content)">复制全文</button>
              <button type="button" @click="saveAssistantMarkdown(message, index)">保存 Markdown</button>
              <button type="button" @click="saveAssistantText(message, index)">保存 TXT</button>
              <button type="button" :disabled="isSending" @click="regenerateMessage(index)">{{ copy.regenerate }}</button>
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
          <button v-if="isSending" class="stop-button" type="button" aria-label="停止生成" title="停止生成" @click="stopGeneration"><span></span>停止</button>
          <button v-else class="send-button" type="button" :disabled="!canSend" :aria-label="copy.send" @click="sendMessage()">&#8593;</button>
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
