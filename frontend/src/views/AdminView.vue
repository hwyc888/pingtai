<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BrandMark from '../components/BrandMark.vue'

const router = useRouter()
const loading = ref(true)
const saving = ref(false)
const mode = ref('local')
const users = ref([])
const total = ref(0)
const providers = ref([])
const selectedProviderId = ref(0)
const providerModels = ref([])
const errorMessage = ref('')
const successMessage = ref('')
const username = ref('')
const password = ref('')
const userType = ref(0)
const resetUserId = ref(0)
const resetPassword = ref('')
const providerForm = ref({
  id: 0,
  name: '',
  base_url: 'https://api.openai.com/v1',
  api_key: '',
  default_model: '',
  max_output_tokens: 0,
  timeout_seconds: 180,
  system_prompt: '',
})

function assertAdminResponse(result) {
  if (Number(result?.errcode) !== 200) throw new Error(result?.errmsg || '请求失败')
  return result
}

function assertProviderResponse(result) {
  if (Number(result?.code) !== 0 && Number(result?.code) !== 200) {
    throw new Error(result?.msg || '供应商请求失败')
  }
  return result
}

function openSetup() {
  window.location.assign('/setup')
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const profileResult = await api.get('/auth/me')
    const profile = profileResult?.data || {}
    if (!profile.is_admin) {
      errorMessage.value = '当前账号没有管理员权限。'
      return
    }
    const modeResult = await api.get('/auth/mode', undefined, { auth: false })
    mode.value = modeResult?.data?.mode || 'local'
    await Promise.all([loadProviders(), mode.value === 'local' ? loadUsers() : Promise.resolve()])
  } catch (error) {
    errorMessage.value = error?.message || '无法加载管理数据'
  } finally {
    loading.value = false
  }
}

async function loadUsers() {
  const result = assertAdminResponse(await api.get('/auth/get_user_list', { pagenum: 1, pagesize: 50 }))
  users.value = Array.isArray(result.data) ? result.data : []
  total.value = Number(result.total || users.value.length)
}

async function loadProviders() {
  const result = assertProviderResponse(await api.get('/admin/providers'))
  providers.value = Array.isArray(result.data) ? result.data : []
  const current = providers.value.find((item) => Number(item.id) === Number(selectedProviderId.value))
  if (current) {
    providerModels.value = current.models || []
  } else if (providers.value.length) {
    selectProvider(providers.value[0])
  } else {
    newProvider()
  }
}

function newProvider() {
  selectedProviderId.value = 0
  providerModels.value = []
  providerForm.value = {
    id: 0, name: '', base_url: 'https://api.openai.com/v1', api_key: '',
    default_model: '', max_output_tokens: 0, timeout_seconds: 180, system_prompt: '',
  }
}

function selectProvider(provider) {
  selectedProviderId.value = Number(provider.id)
  providerModels.value = (provider.models || []).map((item) => ({ ...item }))
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
}

async function saveProvider() {
  if (!providerForm.value.name.trim() || !providerForm.value.base_url.trim()) {
    errorMessage.value = '请填写供应商名称和 Base URL。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const result = assertProviderResponse(await api.post('/admin/providers/save', {
      ...providerForm.value,
      id: Number(providerForm.value.id || 0),
      max_output_tokens: Number(providerForm.value.max_output_tokens || 0),
      timeout_seconds: Number(providerForm.value.timeout_seconds || 180),
    }))
    selectedProviderId.value = Number(result.data.id)
    providerForm.value.id = selectedProviderId.value
    successMessage.value = '供应商配置已保存。'
    await loadProviders()
  } catch (error) {
    errorMessage.value = error?.message || '保存供应商失败'
  } finally {
    saving.value = false
  }
}

async function discoverModels() {
  if (!selectedProviderId.value) {
    errorMessage.value = '请先保存供应商，再获取模型。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const result = assertProviderResponse(await api.post('/admin/providers/discover', { id: selectedProviderId.value }))
    providerModels.value = Array.isArray(result.data?.models) ? result.data.models : []
    if (!providerForm.value.default_model && providerModels.value.length) {
      providerForm.value.default_model = providerModels.value[0].model_id
    }
    const savedProvider = providers.value.find((item) => Number(item.id) === selectedProviderId.value)
    if (savedProvider) savedProvider.models = providerModels.value
    successMessage.value = '已获取 ' + providerModels.value.length + ' 个模型。上下文长度由上游提供时自动填入，否则可手动补充。'
  } catch (error) {
    errorMessage.value = error?.message || '获取上游模型失败'
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  if (!selectedProviderId.value) {
    errorMessage.value = '请先保存供应商，再测试连接。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const result = assertProviderResponse(await api.post('/admin/providers/test', { id: selectedProviderId.value }))
    const data = result.data || {}
    successMessage.value = '连接成功：HTTP ' + data.status_code + '，延迟 ' + data.latency_ms + ' ms，上游返回 ' + data.model_count + ' 个模型。'
  } catch (error) {
    errorMessage.value = error?.message || '供应商连接测试失败'
  } finally {
    saving.value = false
  }
}

async function testModel() {
  if (!selectedProviderId.value || !providerForm.value.default_model) {
    errorMessage.value = '请先选择一个默认模型。'
    return
  }
  if (!window.confirm('将向上游发送一条简短测试请求，最多请求 8 个输出 Token，可能产生少量 API 费用。是否继续？')) return
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const result = assertProviderResponse(await api.post('/admin/providers/test-model', {
      id: selectedProviderId.value,
      model_id: providerForm.value.default_model,
    }))
    const data = result.data || {}
    successMessage.value = '模型调用成功：' + data.model_id + '，延迟 ' + data.latency_ms + ' ms。返回：' + (data.response_preview || '（无文本预览）')
  } catch (error) {
    errorMessage.value = error?.message || '模型可用性测试失败'
  } finally {
    saving.value = false
  }
}

async function saveModelMetadata(model) {
  saving.value = true
  errorMessage.value = ''
  try {
    assertProviderResponse(await api.post('/admin/providers/models/update', {
      provider_id: selectedProviderId.value,
      model_id: model.model_id,
      context_length: model.context_length ? Number(model.context_length) : null,
      max_output_tokens: model.max_output_tokens ? Number(model.max_output_tokens) : null,
      enabled: Boolean(model.enabled),
    }))
    successMessage.value = '模型 ' + model.model_id + ' 配置已保存。'
    await loadProviders()
  } catch (error) {
    errorMessage.value = error?.message || '保存模型参数失败'
  } finally {
    saving.value = false
  }
}

async function activateProvider() {
  if (!selectedProviderId.value || !providerForm.value.default_model) {
    errorMessage.value = '请选择供应商和默认模型。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  try {
    assertProviderResponse(await api.post('/admin/providers/activate', { id: selectedProviderId.value, default_model: providerForm.value.default_model }))
    successMessage.value = '默认供应商已切换。请重启 pingtai 服务后生效。'
    await loadProviders()
  } catch (error) {
    errorMessage.value = error?.message || '启用供应商失败'
  } finally {
    saving.value = false
  }
}

async function deleteProvider() {
  if (!selectedProviderId.value || !window.confirm('确定删除当前供应商配置吗？')) return
  saving.value = true
  try {
    assertProviderResponse(await api.post('/admin/providers/delete', { id: selectedProviderId.value }))
    successMessage.value = '供应商已删除。'
    newProvider()
    await loadProviders()
  } catch (error) {
    errorMessage.value = error?.message || '删除供应商失败'
  } finally {
    saving.value = false
  }
}

async function addUser() {
  if (!username.value.trim() || password.value.length < 8) {
    errorMessage.value = '请输入账号，并设置至少 8 位密码。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    assertAdminResponse(await api.post('/auth/addUser', {
      username: username.value.trim(),
      password: password.value,
      usertype: Number(userType.value),
    }))
    username.value = ''
    password.value = ''
    userType.value = 0
    successMessage.value = '账号已创建。'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error?.message || '创建账号失败'
  } finally {
    saving.value = false
  }
}

async function deleteUser(user) {
  if (!window.confirm('确定删除账号“' + user.username + '”吗？')) return
  errorMessage.value = ''
  successMessage.value = ''
  try {
    assertAdminResponse(await api.post('/auth/delUser', { id: user.id }))
    successMessage.value = '账号已删除。'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error?.message || '删除账号失败'
  }
}

async function savePassword() {
  if (!resetUserId.value || resetPassword.value.length < 8) {
    errorMessage.value = '新密码至少需要 8 位。'
    return
  }
  saving.value = true
  try {
    assertAdminResponse(await api.post('/auth/editUser', { id: resetUserId.value, password: resetPassword.value }))
    resetUserId.value = 0
    resetPassword.value = ''
    successMessage.value = '密码已重置。'
    errorMessage.value = ''
  } catch (error) {
    errorMessage.value = error?.message || '重置密码失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="admin-page">
    <header class="admin-header">
      <div>
        <div class="admin-brand-line"><BrandMark class="admin-brand-mark" /><p class="admin-eyebrow">PINGTAI ADMIN</p></div>
        <h1>平台管理</h1>
        <p class="admin-subtitle">管理员配置 AI 供应商、模型和用户账号；客户端无需选择模型。</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/knowledge')">知识库管理</button>
        <button class="admin-secondary" type="button" @click="openSetup">系统设置（登录/数据库）</button>
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
      </div>
    </header>

    <p v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</p>
    <p v-if="successMessage" class="admin-alert admin-alert-success" role="status">{{ successMessage }}</p>
    <p v-if="loading" class="admin-empty">正在加载管理数据…</p>

    <template v-else-if="!errorMessage">
      <section class="admin-card">
        <div class="admin-card-heading">
          <div>
            <h2>登录认证</h2>
            <p>当前登录方式由管理员配置，普通用户不需要选择登录系统。</p>
          </div>
          <span class="admin-mode">{{ mode === 'sso' ? '学校统一身份认证' : '本地账号密码' }}</span>
        </div>
        <p v-if="mode === 'sso'" class="admin-note">当前使用学校 SSO。平台管理员由 SSO 账号白名单控制；如需调整认证凭据和白名单，请在系统设置页完成。</p>
        <p v-else class="admin-note">当前使用本地账号。可在本页创建普通用户、管理员并重置密码。</p>
        <p class="admin-note">AI 供应商、API Key、模型列表和默认模型统一在本页管理。首次初始化填写的 OpenAI 配置会自动导入，无需在系统设置中重复修改。</p>
      </section>

      <section class="admin-card">
        <div class="admin-card-heading">
          <div>
            <h2>API 供应商</h2>
            <p>配置 OpenAI 兼容 Base URL 和 API Key，获取上游模型并设置平台默认模型。</p>
          </div>
          <button class="admin-secondary" type="button" @click="newProvider">新增供应商</button>
        </div>
        <div class="provider-layout">
          <aside class="provider-list">
            <button v-for="provider in providers" :key="provider.id" type="button"
              class="provider-list-item" :class="{ active: Number(provider.id) === selectedProviderId }"
              @click="selectProvider(provider)">
              <strong>{{ provider.name }}</strong>
              <small>{{ provider.active ? '当前默认供应商' : provider.base_url }}</small>
            </button>
            <p v-if="!providers.length" class="admin-empty">还没有配置供应商</p>
          </aside>

          <div class="provider-editor">
            <div class="provider-form-grid">
              <label>供应商名称<input v-model="providerForm.name" maxlength="80" placeholder="例如：DeepSeek 主站" /></label>
              <label>Base URL<input v-model="providerForm.base_url" placeholder="https://api.example.com/v1" /></label>
              <label class="full">API Key<input v-model="providerForm.api_key" type="password" autocomplete="new-password"
                :placeholder="providers.find((p) => Number(p.id) === selectedProviderId)?.has_api_key ? '已保存，留空保持不变' : '输入上游 API Key'" /></label>
              <label>请求超时（秒）<input v-model.number="providerForm.timeout_seconds" type="number" min="1" max="3600" /></label>
              <label>最大输出 Token（0 = 上游默认）<input v-model.number="providerForm.max_output_tokens" type="number" min="0" /></label>
              <label class="full">系统提示词<textarea v-model="providerForm.system_prompt" rows="2" placeholder="可选，应用于平台网页对话" /></label>
              <label>默认模型<select v-model="providerForm.default_model">
                <option value="">请选择默认模型</option>
                <option v-for="item in providerModels.filter((m) => m.enabled)" :key="item.model_id" :value="item.model_id">{{ item.model_id }}</option>
              </select></label>
            </div>
            <div class="provider-actions">
              <button class="admin-primary" type="button" :disabled="saving" @click="saveProvider">保存供应商</button>
              <button class="admin-secondary" type="button" :disabled="saving || !selectedProviderId" @click="testConnection">测试连接</button>
              <button class="admin-secondary" type="button" :disabled="saving || !selectedProviderId" @click="discoverModels">自动获取模型</button>
              <button class="admin-secondary" type="button" :disabled="saving || !selectedProviderId || !providerForm.default_model" @click="testModel">测试默认模型</button>
              <button class="admin-primary" type="button" :disabled="saving || !selectedProviderId" @click="activateProvider">设为默认供应商</button>
              <button class="admin-text-button danger" type="button" :disabled="saving || !selectedProviderId" @click="deleteProvider">删除</button>
            </div>
            <p class="admin-note">模型上下文长度并非所有兼容接口都会返回；自动发现不到时可以在模型列表中手动填写。上下文长度是供应商元数据，平台不会据此猜测或篡改上游请求。</p>
            <div v-if="providerModels.length" class="admin-table-wrap provider-model-table">
              <table class="admin-table">
                <thead><tr><th>模型 ID</th><th>上下文长度</th><th>最大输出 Token</th><th>启用</th><th>操作</th></tr></thead>
                <tbody>
                  <tr v-for="item in providerModels" :key="item.model_id">
                    <td>{{ item.model_id }}</td>
                    <td><input class="model-metadata-input" type="number" min="1" :value="item.context_length || ''"
                      @change="item.context_length = $event.target.value ? Number($event.target.value) : null" /></td>
                    <td><input class="model-metadata-input" type="number" min="1" :value="item.max_output_tokens || ''"
                      @change="item.max_output_tokens = $event.target.value ? Number($event.target.value) : null" /></td>
                    <td><input type="checkbox" v-model="item.enabled" /></td>
                    <td><button class="admin-text-button" type="button" :disabled="saving" @click="saveModelMetadata(item)">保存</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else class="admin-empty">保存供应商后，点击“自动获取模型”。</p>
          </div>
        </div>
      </section>

      <template v-if="mode === 'local'">
        <section class="admin-card">
          <div class="admin-card-heading">
            <div>
              <h2>添加用户</h2>
              <p>用户登录后直接进入 AI 对话，无需选择模型或填写 API Key。</p>
            </div>
          </div>
          <form class="admin-form" @submit.prevent="addUser">
            <label>账号<input v-model="username" maxlength="50" autocomplete="off" placeholder="输入用户名" required /></label>
            <label>初始密码<input v-model="password" type="password" minlength="8" maxlength="72" autocomplete="new-password" placeholder="至少 8 位" required /></label>
            <label>角色<select v-model.number="userType"><option :value="0">普通用户</option><option :value="1">管理员</option></select></label>
            <button class="admin-primary" type="submit" :disabled="saving">创建账号</button>
          </form>
        </section>

        <section class="admin-card">
          <div class="admin-card-heading">
            <div>
              <h2>用户账号</h2>
              <p>共 {{ total }} 个账号，最多显示最近 50 个。</p>
            </div>
            <button class="admin-secondary" type="button" @click="loadUsers">刷新</button>
          </div>
          <div class="admin-table-wrap">
            <table class="admin-table">
              <thead><tr><th>账号</th><th>角色</th><th>状态</th><th>操作</th></tr></thead>
              <tbody>
                <tr v-for="user in users" :key="user.id">
                  <td>{{ user.username }}</td>
                  <td>{{ Number(user.usertype) === 1 ? '管理员' : '普通用户' }}</td>
                  <td>{{ Number(user.status) === 1 ? '正常' : '停用' }}</td>
                  <td>
                    <div class="admin-row-actions">
                      <button class="admin-text-button" type="button" @click="resetUserId = user.id; resetPassword = ''">重置密码</button>
                      <button class="admin-text-button danger" type="button" @click="deleteUser(user)">删除</button>
                    </div>
                  </td>
                </tr>
                <tr v-if="!users.length"><td colspan="4" class="admin-empty">暂无账号</td></tr>
              </tbody>
            </table>
          </div>
          <form v-if="resetUserId" class="admin-reset-form" @submit.prevent="savePassword">
            <strong>重置账号 ID {{ resetUserId }} 的密码</strong>
            <input v-model="resetPassword" type="password" minlength="8" maxlength="72" autocomplete="new-password" placeholder="输入新密码（至少 8 位）" required />
            <button class="admin-primary" type="submit" :disabled="saving">保存新密码</button>
            <button class="admin-secondary" type="button" @click="resetUserId = 0">取消</button>
          </form>
        </section>
      </template>
    </template>
  </main>
</template>
