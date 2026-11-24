<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
const supportsMessageEdit = ref(false)
const supportsLocalHistory = ref(false)
const editingMessageId = ref('')
const composerInput = ref(null)
const knowledgeBases = ref([])
const knowledgeEnabled = ref(false)
const selectedKnowledgeBaseId = ref('')
const authMode = ref('local')
const selectedBot = ref(null)
const conversationId = ref('')
const messages = ref([])
const conversations = ref([])
const draft = ref('')
const attachments = ref([])
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
const canSend = computed(() => !isSending.value && Boolean(draft.value.trim() || attachments.value.length))
const composerExpanded = computed(() =>
  attachments.value.length > 0 || Boolean(editingMessageId.value) ||
  draft.value.includes('\n') || draft.value.length > 55
)

const selectedKnowledgeBase = computed(() =>
  knowledgeBases.value.find((item) => item.id === selectedKnowledgeBaseId.value) || null
)
const knowledgeMentionSelected = ref(false)
const showMentionMenu = ref(false)
const mentionQuery = ref('')
const mentionStart = ref(-1)
const mentionEnd = ref(-1)
const knowledgeMentionActive = computed(() => knowledgeMentionSelected.value && draft.value.includes('@知识库'))
const showKnowledgeContext = computed(() => knowledgeMentionActive.value && knowledgeBases.value.length > 0)

function fileReference(file) {
  return { name: file.name, size: file.size, type: isImageFile(file) ? 'image' : 'file' }
}

function formatFileSize(bytes) {
  const size = Number(bytes) || 0
  return size >= 1048576 ? (size / 1048576).toFixed(1) + ' MB'
    : size >= 1024 ? (size / 1024).toFixed(1) + ' KB' : size + ' B'
}

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
  supportsMessageEdit.value = Boolean(data.supports_message_edit)
  supportsLocalHistory.value = Boolean(data.supports_local_history ?? data.supports_message_edit)
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
    authMode.value = profile.auth_method || 'local'
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
  editingMessageId.value = ''
  draft.value = ''
  attachments.value = []
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
        attachments: Array.isArray(message.attachments) ? message.attachments : [],
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
      editingMessageId.value = ''
      draft.value = ''
      if (selectedBot.value) await createConversation()
    }
  } catch (error) {
    historyError.value = error?.message || '删除历史对话失败'
  }
}

async function clearCurrentConversation() {
  if (!supportsLocalHistory.value || !conversationId.value || !messages.value.length || isSending.value) return
  if (!window.confirm('确定清空当前页的全部消息吗？当前会话会保留，其他历史对话不会受影响。')) return
  try {
    assertSuccess(await api.post('/agents/clearConversation', { conversation_id: conversationId.value }))
    messages.value = []
    editingMessageId.value = ''
    draft.value = ''
    attachments.value = []
    conversations.value = conversations.value.filter((item) => item.id !== conversationId.value)
    pageError.value = ''
    historyError.value = ''
    scrollToBottom()
  } catch (error) {
    pageError.value = error?.message || '清空当前页失败'
  }
}

async function clearAllHistory() {
  if (!supportsLocalHistory.value || isSending.value) return
  if (!window.confirm('确定清空当前账号的全部历史对话吗？包括当前页在内的所有会话和消息都会被永久删除，且无法恢复。')) return
  try {
    assertSuccess(await api.post('/agents/clearConversations', {}))
    conversations.value = []
    conversationId.value = ''
    messages.value = []
    editingMessageId.value = ''
    draft.value = ''
    attachments.value = []
    pageError.value = ''
    historyError.value = ''
    scrollToBottom()
  } catch (error) {
    historyError.value = error?.message || '清空历史对话失败'
  }
}

async function sendMessage(text = draft.value, options = {}) {
  const rawContent = String(text || '')
  const knowledgeMentionRequested = knowledgeMentionSelected.value &&
    rawContent.includes('@知识库') && knowledgeBases.value.length > 0
  const contentWithoutMention = knowledgeMentionRequested
    ? rawContent.replace(/@知识库/g, ' ')
    : rawContent
  const normalizedContent = contentWithoutMention.replace(/[ \t]{2,}/g, ' ').trim()
  const useKnowledge = knowledgeMentionRequested && knowledgeEnabled.value && Boolean(selectedKnowledgeBaseId.value)
  const content = normalizedContent ||
    (attachments.value.length ? '请阅读并分析附件内容。' : useKnowledge ? '请根据所选知识库回答我的问题。' : '')
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
  const selectedFiles = options.regenerate ? [] : attachments.value.slice()
  if (!conversationId.value) {
    try { await createConversation() } catch (error) {
      pageError.value = error?.message || copy.noConversation
      isSending.value = false
      return
    }
  }
  if (editingMessageId.value && !options.regenerate) {
    const editedIndex = messages.value.findIndex((message) => String(message.id) === editingMessageId.value)
    if (editedIndex < 0) {
      pageError.value = '找不到需要编辑的原消息，请重新打开历史会话。'
      isSending.value = false
      return
    }
    try {
      assertSuccess(await api.post('/agents/editMessage', {
        conversation_id: conversationId.value,
        message_id: editingMessageId.value,
      }))
    } catch (error) {
      pageError.value = error?.message || '编辑消息失败'
      isSending.value = false
      return
    }
    messages.value.splice(editedIndex)
    editingMessageId.value = ''
  }
  if (!options.regenerate) messages.value.push({
    id: 'user-' + Date.now(), role: 'user', content,
    attachments: selectedFiles.map(fileReference),
  })
  const assistantIndex = messages.value.push({
    id: 'assistant-' + Date.now(), role: 'assistant', content: '', reasoning_content: '',
    pending: true, stopped: false,
  }) - 1
  if (!options.regenerate) { draft.value = ''; attachments.value = []; nextTick(resizeComposer) }
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
    form.append('knowledge_base_id', useKnowledge ? selectedKnowledgeBaseId.value : '')
    form.append('url', '')
    selectedFiles.forEach((file) => form.append('files', file))
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
        if (data.user_message_id && messages.value[assistantIndex - 1]?.role === 'user') {
          messages.value[assistantIndex - 1].id = data.user_message_id
        }
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
    if (!options.regenerate && !stopRequested && selectedFiles.length && !attachments.value.length) {
      attachments.value = selectedFiles
    }
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
        if (!options.regenerate) {
          draft.value = rawContent
          attachments.value = selectedFiles
          if (knowledgeMentionRequested) {
            knowledgeMentionSelected.value = true
            knowledgeEnabled.value = true
          }
        }
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

function resizeComposer() {
  const input = composerInput.value
  if (!input) return
  input.style.height = 'auto'
  input.style.height = Math.min(input.scrollHeight, 160) + 'px'
}

function updateMentionMenu() {
  const input = composerInput.value
  if (!input || isSending.value) {
    showMentionMenu.value = false
    return
  }

  const caret = input.selectionStart ?? draft.value.length
  const beforeCaret = draft.value.slice(0, caret)
  const match = beforeCaret.match(/@([^\\s@]*)$/)
  if (!match) {
    showMentionMenu.value = false
    return
  }

  const start = caret - match[0].length
  const previousCharacter = beforeCaret[start - 1] || ''
  if (previousCharacter && /[A-Za-z0-9._%+-]/.test(previousCharacter)) {
    showMentionMenu.value = false
    return
  }

  const query = match[1]
  const normalizedQuery = query.toLocaleLowerCase()
  const matchesKnowledge = !normalizedQuery ||
    '知识库'.startsWith(normalizedQuery) ||
    'knowledge'.startsWith(normalizedQuery) ||
    'kb'.startsWith(normalizedQuery)

  if (!matchesKnowledge) {
    showMentionMenu.value = false
    return
  }

  mentionQuery.value = query
  mentionStart.value = start
  mentionEnd.value = caret
  showMentionMenu.value = true
}

function handleComposerInput() {
  resizeComposer()
  updateMentionMenu()
}

function closeMentionMenu() {
  showMentionMenu.value = false
}

function selectKnowledgeMention() {
  if (isSending.value) return
  if (!knowledgeBases.value.length) {
    showMentionMenu.value = false
    pageError.value = '当前账号暂无可用知识库，请先创建知识库或联系管理员授权。'
    return
  }

  const start = mentionStart.value
  const end = mentionEnd.value
  if (start < 0 || end < start) {
    showMentionMenu.value = false
    return
  }

  const mentionText = '@知识库 '
  draft.value = draft.value.slice(0, start) + mentionText + draft.value.slice(end)
  knowledgeMentionSelected.value = true
  knowledgeEnabled.value = true
  showMentionMenu.value = false

  const nextCaret = start + mentionText.length
  nextTick(() => {
    const input = composerInput.value
    if (!input) return
    input.focus()
    input.setSelectionRange(nextCaret, nextCaret)
    resizeComposer()
  })
}

watch(draft, (value) => {
  if (!value.includes('@知识库')) {
    knowledgeMentionSelected.value = false
    knowledgeEnabled.value = false
  }
  nextTick(resizeComposer)
}, { flush: 'sync' })

function stopGeneration() {
  if (!activeRequestController || !isSending.value) return
  stopRequested = true
  activeRequestController.abort()
}
function addAttachments(files) {
  const selected = Array.from(files || [])
  if (!selected.length) return

  const allowed = ['txt', 'jpg', 'jpeg', 'png']
  const accepted = []
  let rejected = false
  for (const file of selected) {
    const filename = String(file?.name || '')
    const extension = filename.split('.').pop().toLowerCase()
    if (!allowed.includes(extension)) {
      rejected = true
      continue
    }
    const duplicate = attachments.value.some((item) =>
      item.name === filename && item.size === file.size && item.lastModified === file.lastModified
    ) || accepted.some((item) =>
      item.name === filename && item.size === file.size && item.lastModified === file.lastModified
    )
    if (!duplicate) accepted.push(file)
  }

  const available = Math.max(0, 10 - attachments.value.length)
  attachments.value = [...attachments.value, ...accepted.slice(0, available)]
  if (rejected) pageError.value = copy.uploadUnsupported
  else if (accepted.length > available) pageError.value = '单次最多上传 10 个附件。'
  else if (accepted.length) pageError.value = ''
  nextTick(resizeComposer)
}

function handleFileChange(event) {
  addAttachments(event.target.files)
  event.target.value = ''
}

function handleComposerPaste(event) {
  if (isSending.value || !event.clipboardData) return

  const clipboard = event.clipboardData
  const items = Array.from(clipboard.items || [])
  let imageFiles = items
    .filter((item) => item.kind === 'file' && /^image\/(png|jpeg)$/i.test(item.type || ''))
    .map((item) => item.getAsFile())
    .filter(Boolean)

  if (!imageFiles.length) {
    imageFiles = Array.from(clipboard.files || []).filter((file) =>
      /^image\/(png|jpeg)$/i.test(file.type || '') || /\.(png|jpe?g)$/i.test(file.name || '')
    )
  }
  if (!imageFiles.length) return

  event.preventDefault()
  const timestamp = new Date()
  const stamp = [
    timestamp.getFullYear(),
    String(timestamp.getMonth() + 1).padStart(2, '0'),
    String(timestamp.getDate()).padStart(2, '0'),
    '-',
    String(timestamp.getHours()).padStart(2, '0'),
    String(timestamp.getMinutes()).padStart(2, '0'),
    String(timestamp.getSeconds()).padStart(2, '0'),
  ].join('')

  const normalizedFiles = imageFiles.map((file, index) => {
    const type = String(file.type || '').toLowerCase()
    const extension = type === 'image/jpeg' || /\.jpe?g$/i.test(file.name || '') ? 'jpg' : 'png'
    const originalName = String(file.name || '').trim()
    const hasUsefulName = originalName && !/^(image|blob|clipboard)(\.[a-z0-9]+)?$/i.test(originalName)
    const name = hasUsefulName && /\.(png|jpe?g)$/i.test(originalName)
      ? originalName
      : '截图-' + stamp + (imageFiles.length > 1 ? '-' + (index + 1) : '') + '.' + extension
    try {
      return new File([file], name, { type: type || (extension === 'jpg' ? 'image/jpeg' : 'image/png'), lastModified: Date.now() })
    } catch {
      return file
    }
  })

  addAttachments(normalizedFiles)
}

function removeAttachment(index) {
  attachments.value = attachments.value.filter((_, itemIndex) => itemIndex !== index)
}

function handleComposerKeydown(event) {
  if (showMentionMenu.value) {
    if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault()
      selectKnowledgeMention()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMentionMenu()
      return
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      return
    }
  }

  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    sendMessage()
  }
}

async function copyMessage(content) { await copyText(content) }

function beginEditMessage(index) {
  if (!supportsMessageEdit.value || isSending.value) return
  const message = messages.value[index]
  if (!message || message.role !== 'user' || message.id == null) return
  if (message.attachments?.length) {
    pageError.value = '含附件的消息请重新上传文件后编辑重发。'
    return
  }
  editingMessageId.value = String(message.id)
  draft.value = message.content || ''
  pageError.value = ''
  nextTick(() => { composerInput.value?.focus(); resizeComposer() })
}

function cancelEditMessage() {
  editingMessageId.value = ''
  draft.value = ''
  nextTick(() => { composerInput.value?.blur(); resizeComposer() })
}

function regenerateMessage(index) {
  const previousUser = [...messages.value.slice(0, index)].reverse().find((item) => item.role === 'user')
  if (previousUser?.attachments?.length) {
    pageError.value = '该消息含附件，请重新上传文件后提问，以免模型遗漏附件内容。'
    return
  }
  if (previousUser) sendMessage(previousUser.content, { regenerate: true })
}

async function startNewConversation() {
  if (isSending.value) return
  editingMessageId.value = ''
  messages.value = []
  draft.value = ''
  attachments.value = []
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
      <div class="chat-header-actions">
        <button v-if="supportsLocalHistory && messages.length" class="clear-current-button" type="button" :disabled="isSending" @click="clearCurrentConversation">清空当前页</button>
        <button class="icon-button new-chat-button" type="button" :aria-label="copy.startNewChat" :disabled="isSending" @click="startNewConversation">+</button>
      </div>
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
          <div class="user-message-wrap">
            <div v-if="message.attachments?.length" class="message-file-references" aria-label="本次提问引用的文件">
              <div v-for="(file, fileIndex) in message.attachments" :key="file.name + '-' + fileIndex" class="message-file-reference">
                <span class="file-reference-icon" aria-hidden="true">{{ file.type === 'image' ? '▧' : '▤' }}</span>
                <span class="file-reference-details">
                  <strong :title="file.name">{{ file.name }}</strong>
                  <small>{{ formatFileSize(file.size) }} · 文件引用</small>
                </span>
              </div>
            </div>
            <div v-if="message.content" class="user-bubble">{{ message.content }}</div>
            <div v-if="supportsMessageEdit && !isSending" class="user-message-actions">
              <button type="button" :disabled="message.attachments?.length" :title="message.attachments?.length ? '含附件的提问请重新上传文件后发送' : '编辑并重新发送'" @click="beginEditMessage(index)">编辑</button>
            </div>
          </div>
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
      <Transition name="mention-suggestion">
        <div
          v-if="showMentionMenu"
          id="mention-suggestion-menu"
          class="mention-suggestion-menu"
          role="listbox"
          aria-label="提及功能"
        >
          <div class="mention-suggestion-heading">提及</div>
          <button
            class="mention-suggestion-option"
            type="button"
            role="option"
            :aria-selected="true"
            :disabled="!knowledgeBases.length"
            @pointerdown.prevent
            @click="selectKnowledgeMention"
          >
            <span class="mention-suggestion-icon"><img src="/zhishiku.png" alt="" aria-hidden="true" /></span>
            <span class="mention-suggestion-copy">
              <strong>知识库</strong>
              <small>{{ knowledgeBases.length ? '引用已授权知识库内容' : '当前没有可用知识库' }}</small>
            </span>
            <kbd>{{ knowledgeBases.length ? 'Enter' : '不可用' }}</kbd>
          </button>
        </div>
      </Transition>

      <Transition name="knowledge-context">
        <div v-if="showKnowledgeContext" class="composer-context-toolbar" aria-label="对话上下文设置">
        <div class="composer-context-main">
          <label class="knowledge-toggle" :class="{ active: knowledgeEnabled }">
            <input v-model="knowledgeEnabled" type="checkbox" :disabled="isSending" />
            <img class="knowledge-toggle-icon" src="/zhishiku.png" alt="" aria-hidden="true" />
            <span>知识库</span>
          </label>
          <select
            v-model="selectedKnowledgeBaseId"
            class="knowledge-base-select"
            :disabled="!knowledgeEnabled || isSending"
            aria-label="选择知识库"
          >
            <option v-for="base in knowledgeBases" :key="base.id" :value="base.id">
              {{ base.name }} · {{ base.owner_username }}
            </option>
          </select>
        </div>
        <div class="composer-context-actions">
          <span v-if="knowledgeEnabled && selectedKnowledgeBase" class="knowledge-document-count">
            {{ Number(selectedKnowledgeBase.document_count || 0) }} 份文档 · {{ selectedKnowledgeBase.name }}
          </span>
          <span v-else class="knowledge-context-hint">启用后将在回答中检索相关资料</span>
          <!-- Future voice-input controls belong in this upper context toolbar. -->
        </div>
        </div>
      </Transition>

      <div class="composer-shell" :class="{ 'composer-compact': !composerExpanded, 'composer-expanded': composerExpanded }">
        <div v-if="editingMessageId" class="editing-message-bar">
          <span>正在编辑已发送的问题，重新发送后该消息之后的回答将被替换</span>
          <button type="button" :disabled="isSending" @click="cancelEditMessage">取消编辑</button>
        </div>

        <Transition name="attachment-panel">
          <TransitionGroup
            v-if="attachments.length"
            name="attachment-tile"
            tag="div"
            class="attachment-list"
            aria-label="待发送文件引用"
          >
            <div
              v-for="(attachment, index) in attachments"
              :key="attachment.name + '-' + attachment.lastModified + '-' + index"
              class="attachment-tile"
            >
              <button
                class="attachment-remove"
                type="button"
                :aria-label="'移除 ' + attachment.name"
                :disabled="isSending"
                @click="removeAttachment(index)"
              >&times;</button>
              <span class="attachment-tile-icon" aria-hidden="true">{{ isImageFile(attachment) ? '▧' : '▤' }}</span>
              <span class="attachment-tile-details">
                <strong :title="attachment.name">{{ attachment.name }}</strong>
                <small>{{ isImageFile(attachment) ? '图片' : 'TXT' }} · {{ formatFileSize(attachment.size) }}</small>
              </span>
            </div>
          </TransitionGroup>
        </Transition>

        <div class="composer-main-row" :class="{ 'composer-main-row-expanded': composerExpanded }">
          <label class="attach-button" :title="copy.attach" aria-label="添加附件">
            <input type="file" accept=".txt,.jpg,.jpeg,.png" multiple :disabled="isSending" @change="handleFileChange" />
            <span aria-hidden="true">+</span>
          </label>

          <textarea
            ref="composerInput"
            v-model="draft"
            class="composer-input"
            rows="1"
            :placeholder="attachments.length ? '询问附件内容…' : '发送消息开始提问…'"
            :disabled="isSending"
            :aria-label="copy.sendPlaceholder"
            :aria-expanded="showMentionMenu"
            aria-controls="mention-suggestion-menu"
            @keydown="handleComposerKeydown"
            @input="handleComposerInput"
            @paste="handleComposerPaste"
            @click="updateMentionMenu"
            @blur="closeMentionMenu"
          ></textarea>

          <div v-if="models.length" class="footer-toolbar composer-model-toolbar">
            <label class="model-select-label">
              <span>模型</span>
              <select :value="selectedBot?.bot_id || ''" :disabled="!models.length || isSending" aria-label="选择模型" @change="selectModel($event.target.value)">
                <option v-for="model in models" :key="model.bot_id" :value="model.bot_id">
                  {{ model.name }}{{ model.default ? '（默认）' : '' }}
                </option>
              </select>
            </label>
          </div>

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

    <Transition name="history-drawer">
      <div v-if="showHistoryDrawer" class="drawer-backdrop" @click.self="showHistoryDrawer = false">
        <aside class="history-drawer" :aria-label="copy.history">
          <button class="start-chat-button" type="button" :disabled="isSending" @click="startNewConversation">+ {{ copy.startNewChat }}</button>
          <button v-if="supportsLocalHistory && conversations.length" class="clear-history-button" type="button" :disabled="isSending" @click="clearAllHistory">清空全部历史对话</button>
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
            <button type="button" @click="showHistoryDrawer = false; router.push('/my-knowledge')">知识中心</button>
            <button v-if="isAdmin" type="button" @click="router.push('/admin')">平台管理</button>
            <button type="button" @click="logout">{{ copy.logout }}</button>
          </footer>
        </aside>
      </div>
    </Transition>
  </main>
</template>
