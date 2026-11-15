<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { clearAuth } from '../auth'
import { copy } from '../copy'
import assistantAvatar from '../../assets/ai-RKIJCCzP.png'
import deepIcon from '../../assets/deep-Cjqy5X0s.png'
import gptIcon from '../../assets/gpt-CnQrq-fq.png'
import tongyiIcon from '../../assets/tongyi-Dqrkk6Z-.png'

const router = useRouter()
const username = ref(localStorage.getItem('username') || '\u7528\u6237')
const workspaces = ref([])
const bots = ref([])
const activeWorkspaceId = ref('')
const selectedBot = ref(null)
const conversationId = ref('')
const messages = ref([])
const draft = ref('')
const attachment = ref(null)
const isSending = ref(false)
const isLoading = ref(false)
const pageError = ref('')
const showHistoryDrawer = ref(false)
const messageList = ref(null)
let disposed = false

const usernameInitial = computed(() => (username.value || 'U').trim().slice(0, 1).toUpperCase())
const canSend = computed(() => !isSending.value && Boolean(draft.value.trim()) && Boolean(selectedBot.value))

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

async function loadWorkspaces() {
  const result = await api.get('/datasets/workspacesList')
  const data = assertSuccess(result)
  workspaces.value = Array.isArray(data.workspaces) ? data.workspaces : []
  return workspaces.value
}

async function loadBots(workspaceId) {
  if (!workspaceId) {
    bots.value = []
    selectedBot.value = null
    return []
  }
  const result = await api.get('/agents/getAgentsList', {
    space_id: workspaceId,
    page_num: 1,
    page_size: 100,
  })
  const data = assertSuccess(result)
  bots.value = Array.isArray(data.space_bots) ? data.space_bots : []
  if (!bots.value.some((bot) => bot.bot_id === selectedBot.value?.bot_id)) {
    selectedBot.value = bots.value[0] || null
  }
  return bots.value
}

async function createConversation() {
  const payload = selectedBot.value?.bot_id ? { bot_id: selectedBot.value.bot_id } : {}
  const result = await api.post('/agents/createConversation', payload)
  const data = assertSuccess(result)
  if (!data.id) throw new Error(copy.noConversation)
  conversationId.value = data.id
  return data.id
}

async function initialize() {
  isLoading.value = true
  pageError.value = ''
  try {
    const profile = assertSuccess(await api.get('/auth/me'))
    username.value = profile.display_name || profile.account || '用户'
    isAdmin.value = Boolean(profile.is_admin)
    localStorage.setItem('username', username.value)
    await loadWorkspaces()
    if (workspaces.value.length) {
      activeWorkspaceId.value = workspaces.value[0].id
      await loadBots(activeWorkspaceId.value)
    }
    await createConversation()
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

function wait(milliseconds) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}

async function pollForChat(chatId, assistantIndex) {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    if (disposed) return
    await wait(1500)
    const detailResult = await api.get('/agents/getChatDetail', {
      conversation_id: conversationId.value,
      chat_id: chatId,
    })
    const detail = assertSuccess(detailResult)
    if (detail.status === 'failed') throw new Error(copy.requestFailed)
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

async function sendMessage(text = draft.value, options = {}) {
  const content = String(text || '').trim()
  if (!content || isSending.value) return
  if (!selectedBot.value?.bot_id) {
    pageError.value = copy.providerUnavailable
    return
  }

  pageError.value = ''
  isSending.value = true
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
    form.append('up_type', selectedFile ? (isImageFile(selectedFile) ? 'image' : 'file') : '')
    form.append('url', '')
    if (selectedFile) form.append('file', selectedFile)

    const result = await api.post('/agents/chat', form)
    const data = assertSuccess(result)
    const chatId = data.id || data.chat_id
    if (!chatId) throw new Error(result?.msg || copy.requestFailed)
    await pollForChat(chatId, assistantIndex)
  } catch (error) {
    if (messages.value[assistantIndex]) {
      messages.value[assistantIndex] = {
        ...messages.value[assistantIndex],
        content: error?.message || copy.requestFailed,
        pending: false,
        failed: true,
      }
    }
    pageError.value = error?.message || copy.requestFailed
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
  const allowed = ['doc', 'docx', 'txt', 'pdf', 'jpg', 'jpeg', 'png']
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
  try {
    await createConversation()
  } catch (error) {
    conversationId.value = ''
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
        <span class="header-mark">AI</span>
        <span>{{ copy.newChat }}</span>
      </div>
      <button class="icon-button new-chat-button" type="button" :aria-label="copy.startNewChat" :disabled="isSending" @click="startNewConversation">+</button>
    </header>

    <section ref="messageList" class="message-list" aria-live="polite">
      <div v-if="!messages.length && !isLoading" class="welcome-panel">
        <div class="welcome-mark">AI</div>
        <h1>{{ copy.emptyTitle }}</h1>
        <p>{{ copy.emptyDescription }}</p>
      </div>
      <div v-if="isLoading" class="loading-state">{{ copy.loading }}</div>

      <article v-for="(message, index) in messages" :key="message.id || index" class="message-row" :class="message.role === 'user' ? 'message-row-user' : 'message-row-assistant'">
        <template v-if="message.role === 'user'">
          <div class="user-bubble">{{ message.content }}</div>
        </template>
        <template v-else>
          <div class="assistant-avatar"><img :src="assistantAvatar" alt="AI" /></div>
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
            <input type="file" accept=".doc,.docx,.txt,.pdf,.jpg,.jpeg,.png" :disabled="isSending" @change="handleFileChange" />
            <span aria-hidden="true">+</span>
          </label>
          <button class="send-button" type="button" :disabled="!canSend" :aria-label="copy.send" @click="sendMessage()">&#8593;</button>
        </div>
      </div>

    </footer>

    <div v-if="showHistoryDrawer" class="drawer-backdrop" @click.self="showHistoryDrawer = false">
      <aside class="history-drawer" :aria-label="copy.history">
        <button class="start-chat-button" type="button" :disabled="isSending" @click="startNewConversation">+ {{ copy.startNewChat }}</button>
        <div class="history-empty-state">
          <span class="history-empty-icon">&#128196;</span>
          <strong>{{ copy.noHistory }}</strong>
          <small>{{ copy.historyHint }}</small>
        </div>
        <footer class="drawer-user">
          <span class="user-avatar">{{ usernameInitial }}</span>
          <strong>{{ username }}</strong>
          <button v-if="isAdmin" type="button" @click="router.push('/admin')">管理后台</button>
          <button type="button" @click="logout">{{ copy.logout }}</button>
        </footer>
      </aside>
    </div>
  </main>
</template>