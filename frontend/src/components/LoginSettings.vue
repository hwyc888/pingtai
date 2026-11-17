<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api'

const loading = ref(true)
const saving = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const users = ref([])
const total = ref(0)
const username = ref('')
const password = ref('')
const userType = ref(0)
const resetUserId = ref(0)
const resetPassword = ref('')

const settings = reactive({
  mode: 'local',
  local_login_enabled: true,
  sso_login_enabled: false,
  sso_base_url: 'https://ids.zcvc.cn/cas',
  sso_service_url: window.location.origin + '/',
  sso_client_id: '',
  sso_client_secret: '',
  sso_redirect_url: window.location.origin + '/auth/sso/callback',
  sso_admin_accounts: '',
  has_client_secret: false,
})

function assertSettings(result) {
  const code = Number(result?.code)
  if (result?.error || (Number.isFinite(code) && code !== 0 && code !== 200)) {
    throw new Error(result?.msg || '请求失败')
  }
  return result?.data ?? {}
}

function assertUserResponse(result) {
  if (Number(result?.errcode) !== 200) throw new Error(result?.errmsg || '用户管理请求失败')
  return result
}

async function loadUsers() {
  const result = assertUserResponse(await api.get('/auth/get_user_list', { pagenum: 1, pagesize: 50 }))
  users.value = Array.isArray(result.data) ? result.data : []
  total.value = Number(result.total || users.value.length)
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [authResult] = await Promise.all([
      api.get('/admin/auth-settings'),
      loadUsers(),
    ])
    const data = assertSettings(authResult)
    settings.mode = data.mode || 'local'
    settings.local_login_enabled = data.local_login_enabled !== false
    settings.sso_login_enabled = Boolean(data.sso_login_enabled)
    settings.sso_base_url = data.sso_base_url || 'https://ids.zcvc.cn/cas'
    settings.sso_service_url = data.sso_service_url || window.location.origin + '/'
    settings.sso_client_id = data.sso_client_id || ''
    settings.sso_client_secret = ''
    settings.sso_redirect_url = data.sso_redirect_url || window.location.origin + '/auth/sso/callback'
    settings.sso_admin_accounts = data.sso_admin_accounts || ''
    settings.has_client_secret = Boolean(data.has_client_secret)
  } catch (error) {
    errorMessage.value = error?.message || '无法读取登录设置'
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    if (!settings.local_login_enabled && !settings.sso_login_enabled) {
      throw new Error('至少启用一种登录方式。')
    }
    settings.mode = settings.local_login_enabled && settings.sso_login_enabled
      ? 'multi'
      : settings.sso_login_enabled ? 'sso' : 'local'
    const data = assertSettings(await api.post('/admin/auth-settings', {
      mode: settings.mode,
      local_login_enabled: settings.local_login_enabled,
      sso_login_enabled: settings.sso_login_enabled,
      sso_base_url: settings.sso_base_url.trim(),
      sso_service_url: settings.sso_service_url.trim(),
      sso_client_id: settings.sso_client_id.trim(),
      sso_client_secret: settings.sso_client_secret,
      sso_redirect_url: settings.sso_redirect_url.trim(),
      sso_admin_accounts: settings.sso_admin_accounts.trim(),
    }))
    settings.sso_client_secret = ''
    settings.has_client_secret = Boolean(data.has_client_secret)
    successMessage.value = settings.local_login_enabled && settings.sso_login_enabled
      ? '平台账户密码和学校统一身份认证均已启用，用户可在登录首页自行选择。'
      : settings.sso_login_enabled
        ? '已启用学校统一身份认证。用户可在登录首页前往学校认证中心。'
        : '已启用平台账户密码登录。'
  } catch (error) {
    errorMessage.value = error?.message || '保存登录设置失败'
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
    assertUserResponse(await api.post('/auth/addUser', {
      username: username.value.trim(),
      password: password.value,
      usertype: Number(userType.value),
    }))
    username.value = ''
    password.value = ''
    userType.value = 0
    successMessage.value = '本地账号已创建。'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error?.message || '创建账号失败'
  } finally {
    saving.value = false
  }
}

async function deleteUser(user) {
  if (!window.confirm('确定删除账号“' + user.username + '”吗？')) return
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    assertUserResponse(await api.post('/auth/delUser', { id: user.id }))
    successMessage.value = '账号已删除。'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error?.message || '删除账号失败'
  } finally {
    saving.value = false
  }
}

async function savePassword() {
  if (!resetUserId.value || resetPassword.value.length < 8) {
    errorMessage.value = '新密码至少需要 8 位。'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    assertUserResponse(await api.post('/auth/editUser', { id: resetUserId.value, password: resetPassword.value }))
    resetUserId.value = 0
    resetPassword.value = ''
    successMessage.value = '本地账号密码已重置。'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error?.message || '重置密码失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="login-settings">
    <p v-if="errorMessage" class="admin-alert admin-alert-error" role="alert">{{ errorMessage }}</p>
    <p v-if="successMessage" class="admin-alert admin-alert-success" role="status">{{ successMessage }}</p>
    <p v-if="loading" class="admin-empty">正在读取登录设置…</p>

    <template v-else>
      <section class="admin-card">
        <div class="admin-card-heading">
          <div>
            <h2>登录方式</h2>
            <p>选择平台使用本地账户认证，或接入漳州城市职业学院统一身份认证平台。</p>
          </div>
          <span class="admin-mode">{{ settings.local_login_enabled && settings.sso_login_enabled ? '双登录方式' : (settings.sso_login_enabled ? '学校统一认证' : '账户密码登录') }}</span>
        </div>

        <form @submit.prevent="saveSettings">
          <div class="login-mode-options">
            <label class="login-mode-option" :class="{ active: settings.local_login_enabled }">
              <input v-model="settings.local_login_enabled" type="checkbox" />
              <span><strong>平台账户密码登录</strong><small>允许使用管理员创建的本地账号和密码。</small></span>
            </label>
            <label class="login-mode-option" :class="{ active: settings.sso_login_enabled }">
              <input v-model="settings.sso_login_enabled" type="checkbox" />
              <span><strong>学校统一身份认证</strong><small>允许用户从登录首页选择学校统一身份认证。</small></span>
            </label>
          </div>
          <p class="admin-note">两种方式可以同时启用。用户访问首页后自行选择登录方式；至少需要启用一种方式。</p>

          <div v-if="settings.sso_login_enabled" class="login-settings-grid">
            <label>CAS 服务地址
              <input v-model="settings.sso_base_url" type="url" placeholder="https://ids.zcvc.cn/cas" required />
            </label>
            <label>应用访问地址（需在学校后台登记）
              <input v-model="settings.sso_service_url" type="url" placeholder="https://your-domain/" />
            </label>
            <label>Client ID
              <input v-model="settings.sso_client_id" autocomplete="off" placeholder="学校申请接入后提供" required />
            </label>
            <label>Client Secret
              <input v-model="settings.sso_client_secret" type="password" autocomplete="new-password"
                :placeholder="settings.has_client_secret ? '已保存密钥，留空则保持不变' : '学校申请接入后提供'" :required="!settings.has_client_secret" />
            </label>
            <label class="login-field-wide">OAuth2 回调地址（必须与学校登记地址完全一致）
              <input v-model="settings.sso_redirect_url" type="url" placeholder="https://your-domain/auth/sso/callback" required />
            </label>
            <label class="login-field-wide">学校管理员账号
              <textarea v-model="settings.sso_admin_accounts" rows="2" placeholder="填写学校统一身份认证中的账号，多个账号用逗号分隔" required></textarea>
            </label>
          </div>

          <p v-if="settings.sso_login_enabled" class="admin-note">
            接入方式采用 PDF 文档中的 OAuth2 授权码模式。请先向学校管理员登记应用访问地址和回调地址；回调地址必须与登记信息完全一致。学校管理员账号应填写统一身份认证返回的账号标识。启用后，用户可在登录首页自行选择学校统一身份认证。
          </p>
          <p v-else class="admin-note">本地账户由平台管理员创建和管理。关闭学校统一身份认证不会删除已配置的 SSO 信息；关闭本地登录也不会删除已有账号。</p>

          <div class="admin-row-actions login-settings-actions">
            <button class="admin-primary" type="submit" :disabled="saving">{{ saving ? '保存中…' : '保存登录设置' }}</button>
          </div>
        </form>
      </section>

      <section class="admin-card">
        <div class="admin-card-heading">
          <div>
            <h2>本地账户管理</h2>
            <p>共 {{ total }} 个本地账号。是否允许本地账号登录由上方开关控制，关闭登录不会删除账号。</p>
          </div>
          <button class="admin-secondary" type="button" :disabled="saving" @click="loadUsers">刷新</button>
        </div>
        <form class="admin-form" @submit.prevent="addUser">
          <label>账号<input v-model="username" maxlength="50" autocomplete="off" placeholder="输入用户名" required /></label>
          <label>初始密码<input v-model="password" type="password" minlength="8" maxlength="72" autocomplete="new-password" placeholder="至少 8 位" required /></label>
          <label>角色<select v-model.number="userType"><option :value="0">普通用户</option><option :value="1">管理员</option></select></label>
          <button class="admin-primary" type="submit" :disabled="saving">创建账号</button>
        </form>
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
                    <button class="admin-text-button danger" type="button" :disabled="saving" @click="deleteUser(user)">删除</button>
                  </div>
                </td>
              </tr>
              <tr v-if="!users.length"><td colspan="4" class="admin-empty">暂无本地账号</td></tr>
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
  </div>
</template>
