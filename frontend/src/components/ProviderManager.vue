<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'

const presets = [
  { id: 'openai', name: 'OpenAI', category: '国际服务', base_url: 'https://api.openai.com/v1', description: 'OpenAI 官方 API', website: 'https://platform.openai.com', mark: 'O', requiresKey: true },
  { id: 'deepseek', name: 'DeepSeek', category: '国内服务', base_url: 'https://api.deepseek.com', description: 'DeepSeek 官方模型服务', website: 'https://platform.deepseek.com', mark: 'D', requiresKey: true },
  { id: 'qwen', name: '通义千问', category: '国内服务', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', description: '阿里云百炼 / DashScope', website: 'https://bailian.console.aliyun.com', mark: 'Q', requiresKey: true },
  { id: 'zhipu', name: '智谱 GLM', category: '国内服务', base_url: 'https://open.bigmodel.cn/api/paas/v4', description: '智谱 AI 开放平台', website: 'https://open.bigmodel.cn', mark: '智', requiresKey: true },
  { id: 'kimi', name: 'Moonshot / Kimi', category: '国内服务', base_url: 'https://api.moonshot.cn/v1', description: '月之暗面 Kimi API', website: 'https://platform.moonshot.cn', mark: 'K', requiresKey: true },
  { id: 'minimax', name: 'MiniMax', category: '国内服务', base_url: 'https://api.minimaxi.com/v1', description: 'MiniMax 国内站 API', website: 'https://platform.minimaxi.com', mark: 'M', requiresKey: true },
  { id: 'volcengine', name: '火山方舟 / 豆包', category: '国内服务', base_url: 'https://ark.cn-beijing.volces.com/api/v3', description: '火山引擎方舟推理服务', website: 'https://console.volcengine.com/ark', mark: '火', requiresKey: true },
  { id: 'siliconflow', name: '硅基流动', category: '聚合平台', base_url: 'https://api.siliconflow.cn/v1', description: '多模型聚合 API', website: 'https://cloud.siliconflow.cn', mark: 'S', requiresKey: true },
  { id: 'openrouter', name: 'OpenRouter', category: '聚合平台', base_url: 'https://openrouter.ai/api/v1', description: '国际模型聚合 API', website: 'https://openrouter.ai', mark: 'R', requiresKey: true },
  { id: 'ollama', name: 'Ollama（本地）', category: '本地部署', base_url: 'http://127.0.0.1:11434/v1', description: '连接部署在平台服务器上的 Ollama', website: 'https://ollama.com', mark: 'O', requiresKey: false },
  { id: 'custom', name: '自定义配置', category: '自定义', base_url: '', description: '填写任意 OpenAI 兼容 API 地址', website: '', mark: '+', requiresKey: false },
]
const categories = ['全部', '国内服务', '国际服务', '聚合平台', '本地部署', '自定义']

const providers = ref([])
const providerView = ref('list')
const selectedPreset = ref(null)
const isAdding = ref(false)
const loading = ref(true)
const saving = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const presetSearch = ref('')
const presetCategory = ref('全部')
const modelSearch = ref('')
const manualModelId = ref('')
const showAdvanced = ref(false)
const showApiKey = ref(false)
const providerModels = ref([])
const providerForm = ref(blankProvider())

function blankProvider() {
  return {
    id: 0,
    name: '',
    base_url: '',
    api_key: '',
    default_model: '',
    max_output_tokens: 0,
    timeout_seconds: 180,
    system_prompt: '',
  }
}

const activeProvider = computed(() => providers.value.find((item) => item.active) || null)
const currentSavedProvider = computed(() => providers.value.find((item) => Number(item.id) === Number(providerForm.value.id)) || null)
const hasSavedKey = computed(() => Boolean(currentSavedProvider.value?.has_api_key))
const filteredPresets = computed(() => {
  const query = presetSearch.value.trim().toLowerCase()
  return presets.filter((preset) => {
    const categoryMatch = presetCategory.value === '全部' || preset.category === presetCategory.value
    const queryMatch = !query || [preset.name, preset.base_url, preset.description].join(' ').toLowerCase().includes(query)
    return categoryMatch && queryMatch
  })
})
const filteredModels = computed(() => {
  const query = modelSearch.value.trim().toLowerCase()
  if (!query) return providerModels.value
  return providerModels.value.filter((item) => item.model_id.toLowerCase().includes(query))
})
const enabledModelCount = computed(() => providerModels.value.filter((item) => item.enabled).length)

function unwrap(result) {
  const code = Number(result?.code)
  if (result?.error || (Number.isFinite(code) && ![0, 200].includes(code))) {
    throw new Error(result?.msg || '供应商请求失败')
  }
  return result?.data ?? result ?? {}
}

function providerPresetFor(item) {
  return presets.find((preset) => preset.id !== 'custom' && preset.base_url === item.base_url) || presets[presets.length - 1]
}

async function loadProviders() {
  const data = unwrap(await api.get('/admin/providers'))
  providers.value = Array.isArray(data) ? data : []
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    await loadProviders()
  } catch (error) {
    errorMessage.value = error?.message || '读取供应商列表失败'
  } finally {
    loading.value = false
  }
}

function openAddProvider() {
  errorMessage.value = ''
  successMessage.value = ''
  isAdding.value = true
  selectedPreset.value = null
  providerForm.value = blankProvider()
  providerModels.value = []
  presetSearch.value = ''
  presetCategory.value = '全部'
  modelSearch.value = ''
  showAdvanced.value = false
  providerView.value = 'presets'
}

function choosePreset(preset) {
  selectedPreset.value = preset
  providerForm.value = {
    ...blankProvider(),
    name: preset.id === 'custom' ? '' : preset.name,
    base_url: preset.base_url,
  }
  providerModels.value = []
  modelSearch.value = ''
  showAdvanced.value = false
  showApiKey.value = false
  providerView.value = 'form'
}

function editProvider(provider) {
  selectedPreset.value = providerPresetFor(provider)
  isAdding.value = false
  providerForm.value = {
    id: Number(provider.id),
    name: provider.name || '',
    base_url: provider.base_url || '',
    api_key: '',
    default_model: provider.default_model || '',
    max_output_tokens: Number(provider.max_output_tokens || 0),
    timeout_seconds: Number(provider.timeout_seconds || 180),
    system_prompt: provider.system_prompt || '',
  }
  providerModels.value = (provider.models || []).map((item) => ({ ...item }))
  modelSearch.value = ''
  showAdvanced.value = false
  showApiKey.value = false
  errorMessage.value = ''
  successMessage.value = ''
  providerView.value = 'form'
}

function backFromForm() {
  errorMessage.value = ''
  successMessage.value = ''
  providerView.value = isAdding.value ? 'presets' : 'list'
}

function backToList() {
  errorMessage.value = ''
  providerView.value = 'list'
}

async function persistProvider() {
  const form = providerForm.value
  if (!form.name.trim()) throw new Error('请填写供应商名称。')
  if (!form.base_url.trim()) throw new Error('请填写 API 请求地址。')
  if (isAdding.value && selectedPreset.value?.requiresKey && !form.api_key.trim()) {
    throw new Error('请填写 API Key。')
  }
  const result = unwrap(await api.post('/admin/providers/save', {
    ...form,
    id: Number(form.id || 0),
    max_output_tokens: Number(form.max_output_tokens || 0),
    timeout_seconds: Number(form.timeout_seconds || 180),
  }))
  const id = Number(result?.id || result?.data?.id)
  if (!id) throw new Error('供应商已提交，但没有收到有效的供应商 ID。')
  providerForm.value.id = id
  isAdding.value = false
  await loadProviders()
  return id
}

async function saveProvider() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const wasActive = Boolean(currentSavedProvider.value?.active)
    await persistProvider()
    successMessage.value = wasActive
      ? '供应商配置已保存。当前正在使用此供应商，请重启服务使修改生效。'
      : '供应商已保存。接下来可以获取模型并设置默认模型。'
  } catch (error) {
    errorMessage.value = error?.message || '保存供应商失败'
  } finally {
    saving.value = false
  }
}

async function discoverModels() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const id = await persistProvider()
    const data = unwrap(await api.post('/admin/providers/discover', { id }))
    providerModels.value = Array.isArray(data.models) ? data.models.map((item) => ({ ...item })) : []
    if (!providerForm.value.default_model || !providerModels.value.some((item) => item.model_id === providerForm.value.default_model && item.enabled)) {
      providerForm.value.default_model = providerModels.value.find((item) => item.enabled)?.model_id || ''
    }
    await loadProviders()
    successMessage.value = providerModels.value.length
      ? '已获取 ' + providerModels.value.length + ' 个模型。勾选要开放给对话使用的模型，并设置默认模型。'
      : '接口连接成功，但没有返回可用模型。'
  } catch (error) {
    errorMessage.value = error?.message || '获取模型失败，请检查 API Key 和请求地址。'
  } finally {
    saving.value = false
  }
}

function onModelToggle(model) {
  if (!model.enabled && providerForm.value.default_model === model.model_id) {
    providerForm.value.default_model = providerModels.value.find((item) => item.enabled)?.model_id || ''
  }
  if (model.enabled && !providerForm.value.default_model) {
    providerForm.value.default_model = model.model_id
  }
}

function setDefaultModel(model) {
  if (!model.enabled) return
  providerForm.value.default_model = model.model_id
}

function selectAllModels(enabled) {
  providerModels.value.forEach((item) => { item.enabled = enabled })
  if (!enabled) {
    providerForm.value.default_model = ''
  } else if (!providerForm.value.default_model) {
    providerForm.value.default_model = providerModels.value[0]?.model_id || ''
  }
}

async function saveModelConfiguration() {
  if (!providerForm.value.id) throw new Error('请先保存供应商。')
  const enabled = providerModels.value.filter((item) => item.enabled)
  if (!enabled.length) throw new Error('至少启用一个模型，才能用于平台对话。')
  if (!enabled.some((item) => item.model_id === providerForm.value.default_model)) {
    providerForm.value.default_model = enabled[0].model_id
  }
  await persistProvider()
  for (const item of providerModels.value) {
    await api.post('/admin/providers/models/update', {
      provider_id: providerForm.value.id,
      model_id: item.model_id,
      context_length: item.context_length ? Number(item.context_length) : null,
      max_output_tokens: item.max_output_tokens ? Number(item.max_output_tokens) : null,
      enabled: Boolean(item.enabled),
    })
  }
  await loadProviders()
}

async function saveModels() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await saveModelConfiguration()
    successMessage.value = '模型选择和默认模型已保存。'
  } catch (error) {
    errorMessage.value = error?.message || '保存模型设置失败'
  } finally {
    saving.value = false
  }
}

async function addManualModel() {
  const modelID = manualModelId.value.trim()
  if (!providerForm.value.id) {
    errorMessage.value = '请先保存供应商配置。'
    return
  }
  if (!modelID) {
    errorMessage.value = '请输入模型 ID。'
    return
  }
  if (providerModels.value.some((item) => item.model_id === modelID)) {
    const existing = providerModels.value.find((item) => item.model_id === modelID)
    existing.enabled = true
    if (!providerForm.value.default_model) providerForm.value.default_model = modelID
    manualModelId.value = ''
    successMessage.value = '该模型已存在，已重新启用。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.post('/admin/providers/models/update', {
      provider_id: providerForm.value.id,
      model_id: modelID,
      context_length: null,
      max_output_tokens: null,
      enabled: true,
    })
    providerModels.value.push({
      model_id: modelID,
      context_length: null,
      max_output_tokens: null,
      enabled: true,
    })
    if (!providerForm.value.default_model) providerForm.value.default_model = modelID
    manualModelId.value = ''
    await loadProviders()
    successMessage.value = '已添加模型 ' + modelID + '。'
  } catch (error) {
    errorMessage.value = error?.message || '添加模型失败'
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const id = await persistProvider()
    const data = unwrap(await api.post('/admin/providers/test', { id }))
    successMessage.value = '连接成功 · HTTP ' + data.status_code + ' · ' + data.latency_ms + ' ms · ' + data.model_count + ' 个模型'
  } catch (error) {
    errorMessage.value = error?.message || '连接测试失败'
  } finally {
    saving.value = false
  }
}

async function testModel() {
  if (!providerForm.value.default_model) {
    errorMessage.value = '请先获取模型并选择默认模型。'
    return
  }
  if (!window.confirm('将向上游发送一条简短测试请求，可能产生少量 API 费用。是否继续？')) return
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const id = await persistProvider()
    const data = unwrap(await api.post('/admin/providers/test-model', {
      id,
      model_id: providerForm.value.default_model,
    }))
    successMessage.value = '模型调用成功 · ' + data.model_id + ' · ' + data.latency_ms + ' ms'
  } catch (error) {
    errorMessage.value = error?.message || '模型测试失败'
  } finally {
    saving.value = false
  }
}

async function activateProvider() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    if (!providerForm.value.id) throw new Error('请先保存供应商并获取模型。')
    await saveModelConfiguration()
    unwrap(await api.post('/admin/providers/activate', {
      id: providerForm.value.id,
      default_model: providerForm.value.default_model,
    }))
    await loadProviders()
    providerView.value = 'list'
    successMessage.value = '已设为默认供应商。请重启 pingtai 服务后生效。'
  } catch (error) {
    errorMessage.value = error?.message || '启用供应商失败'
  } finally {
    saving.value = false
  }
}

async function activateFromList(provider) {
  if (!provider.default_model || !(provider.models || []).some((item) => item.enabled && item.model_id === provider.default_model)) {
    editProvider(provider)
    errorMessage.value = '请先选择一个已启用的默认模型，再启用供应商。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    unwrap(await api.post('/admin/providers/activate', {
      id: provider.id,
      default_model: provider.default_model,
    }))
    await loadProviders()
    successMessage.value = '已设为默认供应商。请重启 pingtai 服务后生效。'
  } catch (error) {
    errorMessage.value = error?.message || '启用供应商失败'
  } finally {
    saving.value = false
  }
}

async function deleteProvider(provider) {
  if (!window.confirm('确定删除供应商“' + provider.name + '”吗？此操作不可撤销。')) return
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    unwrap(await api.post('/admin/providers/delete', { id: provider.id }))
    await loadProviders()
    successMessage.value = '供应商已删除。'
  } catch (error) {
    errorMessage.value = error?.message || '删除供应商失败'
  } finally {
    saving.value = false
  }
}

function finishEditing() {
  providerView.value = 'list'
  errorMessage.value = ''
  successMessage.value = ''
  loadProviders().catch((error) => { errorMessage.value = error?.message || '刷新供应商失败' })
}

onMounted(load)
</script>

<template>
  <section class="provider-manager">
    <div v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</div>
    <div v-if="successMessage" class="admin-alert admin-alert-success" role="status">{{ successMessage }}</div>

    <template v-if="providerView === 'list'">
      <header class="pm-section-header">
        <div>
          <p class="pm-eyebrow">MODEL PROVIDERS</p>
          <h2>模型供应商</h2>
          <p>统一管理 API 地址、密钥和可用模型。一个供应商可以启用多个模型。</p>
        </div>
        <button class="admin-primary pm-add-button" type="button" @click="openAddProvider">＋ 添加供应商</button>
      </header>

      <div v-if="loading" class="pm-empty-state">正在读取供应商…</div>
      <template v-else>
        <div v-if="activeProvider" class="pm-active-banner">
          <span class="pm-active-dot"></span>
          <span>当前默认供应商</span>
          <strong>{{ activeProvider.name }}</strong>
          <span class="pm-active-model">{{ activeProvider.default_model || '尚未设置默认模型' }}</span>
        </div>

        <div v-if="providers.length" class="pm-provider-grid">
          <article v-for="provider in providers" :key="provider.id" class="pm-provider-card" :class="{ 'is-active': provider.active }">
            <div class="pm-provider-card-top">
              <div class="pm-provider-identity">
                <span class="pm-provider-avatar">{{ (provider.name || 'AI').slice(0, 1).toUpperCase() }}</span>
                <div class="pm-provider-heading">
                  <div class="pm-provider-name-row">
                    <strong>{{ provider.name }}</strong>
                    <span v-if="provider.active" class="pm-status-current">当前使用</span>
                  </div>
                  <span class="pm-provider-url">{{ provider.base_url }}</span>
                </div>
              </div>
              <span class="pm-provider-protocol">OpenAI 兼容</span>
            </div>
            <div class="pm-provider-info">
              <div><span>可用模型</span><strong>{{ (provider.models || []).filter((item) => item.enabled).length }}</strong></div>
              <div class="pm-default-model"><span>默认模型</span><strong>{{ provider.default_model || '未设置' }}</strong></div>
              <div><span>API Key</span><strong>{{ provider.has_api_key ? '已配置' : '未配置' }}</strong></div>
            </div>
            <footer class="pm-provider-actions">
              <button class="pm-action-edit" type="button" @click="editProvider(provider)">编辑配置</button>
              <button v-if="!provider.active" class="pm-action-enable" type="button" :disabled="saving" @click="activateFromList(provider)">设为默认</button>
              <button class="pm-action-delete" type="button" :disabled="saving" @click="deleteProvider(provider)">删除</button>
            </footer>
          </article>
        </div>
        <div v-else class="pm-empty-state pm-no-provider">
          <div class="pm-empty-icon">＋</div>
          <h3>还没有添加模型供应商</h3>
          <p>选择一个预设服务商，填写 API Key，即可获取模型并开始对话。</p>
          <button class="admin-primary" type="button" @click="openAddProvider">添加第一个供应商</button>
        </div>
      </template>
    </template>

    <template v-else-if="providerView === 'presets'">
      <header class="pm-builder-header">
        <button class="pm-back-button" type="button" @click="backToList">← 返回供应商列表</button>
        <div class="pm-step-indicator"><span class="is-current">01 选择预设</span><i></i><span>02 填写配置</span></div>
      </header>
      <div class="pm-builder-title">
        <p class="pm-eyebrow">ADD PROVIDER</p>
        <h2>添加新供应商</h2>
        <p>选择常用服务商，系统会自动填写 API 地址；也可以使用自定义配置。</p>
      </div>
      <div class="pm-preset-toolbar">
        <div class="pm-search-box"><span>⌕</span><input v-model="presetSearch" placeholder="搜索供应商名称或 API 地址" /></div>
        <div class="pm-category-list">
          <button v-for="category in categories" :key="category" type="button"
            :class="{ active: presetCategory === category }" @click="presetCategory = category">{{ category }}</button>
        </div>
      </div>
      <div class="pm-preset-grid">
        <button v-for="preset in filteredPresets" :key="preset.id" type="button" class="pm-preset-card"
          :class="{ 'pm-preset-custom': preset.id === 'custom' }" @click="choosePreset(preset)">
          <span class="pm-preset-mark">{{ preset.mark }}</span>
          <span class="pm-preset-copy"><strong>{{ preset.name }}</strong><small>{{ preset.description }}</small></span>
          <span class="pm-preset-arrow">›</span>
        </button>
      </div>
      <p v-if="!filteredPresets.length" class="pm-empty-state">没有找到匹配的供应商。试试“自定义配置”。</p>
      <p class="pm-compatibility-note">当前平台使用 OpenAI Chat Completions 兼容协议。原生 Anthropic、Gemini 等非兼容接口暂不支持直接接入；请确认服务商提供 OpenAI 兼容地址。</p>
    </template>

    <template v-else>
      <header class="pm-builder-header">
        <button class="pm-back-button" type="button" @click="backFromForm">← {{ isAdding ? '返回预设列表' : '返回供应商列表' }}</button>
        <div class="pm-step-indicator"><span class="is-done">01 选择预设</span><i></i><span class="is-current">02 填写配置</span></div>
      </header>
      <div class="pm-builder-title">
        <p class="pm-eyebrow">{{ isAdding ? 'NEW PROVIDER' : 'EDIT PROVIDER' }}</p>
        <h2>{{ isAdding ? '配置供应商' : '编辑供应商' }}</h2>
        <p>填写 API Key 并确认请求地址，保存后即可获取此供应商支持的模型。</p>
      </div>

      <div class="pm-selected-preset">
        <span class="pm-preset-mark">{{ selectedPreset?.mark || '自' }}</span>
        <div><strong>{{ selectedPreset?.name || '自定义配置' }}</strong><small>{{ selectedPreset?.description || 'OpenAI 兼容 API' }}</small></div>
        <button v-if="isAdding && !providerForm.id" type="button" @click="providerView = 'presets'">更换预设</button>
        <a v-else-if="selectedPreset?.website" :href="selectedPreset.website" target="_blank" rel="noreferrer">服务商官网 ↗</a>
      </div>

      <div class="pm-config-card">
        <div class="pm-form-grid">
          <label class="pm-field">
            <span>供应商名称 <b>*</b></span>
            <input v-model="providerForm.name" maxlength="80" placeholder="例如：DeepSeek 官方" />
          </label>
          <label class="pm-field">
            <span>API Key <b v-if="selectedPreset?.requiresKey && !hasSavedKey">*</b></span>
            <div class="pm-key-input">
              <input v-model="providerForm.api_key" :type="showApiKey ? 'text' : 'password'" autocomplete="new-password"
                :placeholder="hasSavedKey ? '已保存密钥，留空则保持不变' : (selectedPreset?.requiresKey ? '粘贴服务商提供的 API Key' : '无需鉴权时可留空')" />
              <button type="button" @click="showApiKey = !showApiKey">{{ showApiKey ? '隐藏' : '显示' }}</button>
            </div>
            <small>密钥只保存在服务器 SQLite 中，不会返回给浏览器。</small>
          </label>
          <label class="pm-field pm-field-wide">
            <span>API 请求地址 <b>*</b></span>
            <input v-model="providerForm.base_url" spellcheck="false" placeholder="https://api.example.com/v1" />
            <small>填写 API Base URL，不要填写完整的 /chat/completions。系统会自动拼接模型列表和聊天接口路径。</small>
          </label>
        </div>

        <button class="pm-advanced-toggle" type="button" @click="showAdvanced = !showAdvanced">
          <span>{{ showAdvanced ? '▾' : '▸' }} 高级选项</span><small>超时、输出上限和系统提示词</small>
        </button>
        <div v-if="showAdvanced" class="pm-advanced-panel">
          <label class="pm-field">
            <span>请求超时（秒）</span>
            <input v-model.number="providerForm.timeout_seconds" type="number" min="1" max="3600" />
          </label>
          <label class="pm-field">
            <span>最大输出 Token（0 表示由上游决定）</span>
            <input v-model.number="providerForm.max_output_tokens" type="number" min="0" />
          </label>
          <label class="pm-field pm-field-wide">
            <span>系统提示词（可选）</span>
            <textarea v-model="providerForm.system_prompt" rows="3" placeholder="留空则使用平台默认提示词" />
          </label>
        </div>

        <div class="pm-form-actions">
          <button class="admin-secondary" type="button" :disabled="saving" @click="saveProvider">仅保存配置</button>
          <button class="admin-secondary" type="button" :disabled="saving" @click="testConnection">测试连接</button>
          <button class="admin-primary" type="button" :disabled="saving" @click="discoverModels">{{ saving ? '处理中…' : '保存并获取模型' }}</button>
        </div>
        <div v-if="providerForm.id" class="pm-manual-model">
          <div><strong>手动添加模型</strong><small>如果服务商没有提供 /models 接口，可以直接填写模型 ID。</small></div>
          <div class="pm-manual-model-input">
            <input v-model="manualModelId" placeholder="例如：deepseek-chat 或 qwen-plus" @keydown.enter.prevent="addManualModel" />
            <button class="admin-secondary" type="button" :disabled="saving || !manualModelId.trim()" @click="addManualModel">添加模型</button>
          </div>
        </div>
      </div>

      <section v-if="providerModels.length" class="pm-model-panel">
        <header class="pm-model-header">
          <div><h3>模型列表</h3><p>已发现 {{ providerModels.length }} 个模型，已启用 {{ enabledModelCount }} 个</p></div>
          <div class="pm-model-tools">
            <button type="button" @click="selectAllModels(true)">全部启用</button>
            <button type="button" @click="selectAllModels(false)">全部停用</button>
          </div>
        </header>
        <div class="pm-model-toolbar">
          <div class="pm-search-box"><span>⌕</span><input v-model="modelSearch" placeholder="搜索模型 ID" /></div>
          <label class="pm-default-select"><span>默认模型</span>
            <select v-model="providerForm.default_model">
              <option v-for="item in providerModels.filter((model) => model.enabled)" :key="item.model_id" :value="item.model_id">{{ item.model_id }}</option>
            </select>
          </label>
        </div>
        <div class="pm-model-list">
          <label v-for="model in filteredModels" :key="model.model_id" class="pm-model-row">
            <input v-model="model.enabled" type="checkbox" @change="onModelToggle(model)" />
            <span class="pm-model-name">{{ model.model_id }}</span>
            <input v-model.number="model.context_length" class="pm-metadata-input" type="number" min="1" placeholder="上下文" title="上下文长度（可选）" />
            <input v-model.number="model.max_output_tokens" class="pm-metadata-input" type="number" min="1" placeholder="输出上限" title="最大输出 Token（可选）" />
            <button type="button" class="pm-default-star" :class="{ selected: providerForm.default_model === model.model_id }"
              :disabled="!model.enabled" @click.prevent="setDefaultModel(model)" :title="model.enabled ? '设为默认模型' : '请先启用模型'">
              {{ providerForm.default_model === model.model_id ? '默认' : '设为默认' }}
            </button>
          </label>
          <p v-if="!filteredModels.length" class="pm-empty-models">没有匹配的模型。</p>
        </div>
        <footer class="pm-model-footer">
          <span>只有启用的模型会出现在对话模型选择器中。</span>
          <div>
            <button class="admin-secondary" type="button" :disabled="saving" @click="saveModels">保存模型设置</button>
            <button class="admin-secondary" type="button" :disabled="saving || !providerForm.default_model" @click="testModel">测试默认模型</button>
            <button class="admin-primary" type="button" :disabled="saving || !enabledModelCount" @click="activateProvider">保存并设为默认</button>
          </div>
        </footer>
      </section>
    </template>
  </section>
</template>
