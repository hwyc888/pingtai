<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { copy } from '../copy'
import BrandMark from '../components/BrandMark.vue'

const busy = ref(false)
const loading = ref(true)
const needsAdmin = ref(false)
const localLoginEnabled = ref(true)
const ssoLoginEnabled = ref(false)
const loginChoice = ref('')
const ssoError = ref('')
const errorMessage = ref('')
const username = ref('')
const password = ref('')
const confirmPassword = ref('')

async function loadBootstrapStatus() {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.get('/auth/bootstrap/status', undefined, { auth: false })
    if (Number(result?.code) !== 0) throw new Error(result?.msg || '无法读取平台状态')
    needsAdmin.value = Boolean(result?.data?.needs_admin)
    ssoError.value = new URLSearchParams(window.location.search).get('sso_error') || ''
    if (!needsAdmin.value) {
      const modeResult = await api.get('/auth/mode', undefined, { auth: false })
      const auth = modeResult?.data || {}
      localLoginEnabled.value = auth.local_login_enabled !== false
      ssoLoginEnabled.value = Boolean(auth.sso_login_enabled)
      if (!localLoginEnabled.value && !ssoLoginEnabled.value) {
        errorMessage.value = '平台暂未启用任何登录方式，请联系管理员。'
      } else if (ssoError.value && ssoLoginEnabled.value) {
        loginChoice.value = localLoginEnabled.value ? 'sso' : ''
        const ssoMessages = {
          '1': '学校统一身份认证未完成。请检查服务日志中的 [sso] 错误记录。',
          config: '统一身份认证配置不完整，请管理员检查 CAS 地址、Client ID、Client Secret 和回调地址。',
          provider: '学校认证中心拒绝了授权请求。请检查学校后台应用登记、Client ID 和回调地址。',
          state: '认证返回时校验状态丢失。请使用同一个 IP/域名和协议重新登录，并确认浏览器允许 Cookie。',
          missing_code: '认证中心没有返回授权码。请检查授权回调登记及学校端授权结果。',
          token: '授权码换取访问令牌失败。请管理员检查 Client Secret、回调地址是否完全一致、服务器时间，以及授权码是否过期或重复使用。',
          profile: '访问令牌已获取，但读取学校用户资料失败。请管理员检查认证中心用户资料接口及服务器网络。',
          identity: '学校用户资料缺少可用账号标识。请检查 /oauth2.0/profile 返回的 account、useraccount 或 id 字段。',
          session: '学校认证已通过，但平台创建登录会话失败。请管理员检查平台数据库和服务日志。',
        }
        errorMessage.value = ssoMessages[ssoError.value] || ssoMessages['1']
      } else if (!localLoginEnabled.value && ssoLoginEnabled.value) {
        loginChoice.value = 'sso'
      } else if (localLoginEnabled.value && !ssoLoginEnabled.value) {
        loginChoice.value = 'local'
      }
    }
  } catch (error) {
    errorMessage.value = error?.message || '暂时无法连接平台，请稍后重试。'
  } finally {
    loading.value = false
  }
}

function chooseLogin(method) {
  loginChoice.value = method
  errorMessage.value = ''
}

function backToLoginChoices() {
  loginChoice.value = ''
  errorMessage.value = ''
}

function startSSO() {
  if (!ssoLoginEnabled.value) return
  window.location.assign('/auth/sso/login')
}

async function submit(event) {
  event.preventDefault()
  if (busy.value) return
  if (!needsAdmin.value && !localLoginEnabled.value) {
    errorMessage.value = '平台账户密码登录未启用，请返回选择其他登录方式。'
    return
  }
  const account = username.value.trim()
  if (!account || !password.value) {
    errorMessage.value = '请输入账号和密码。'
    return
  }
  if (needsAdmin.value && password.value !== confirmPassword.value) {
    errorMessage.value = '两次输入的密码不一致。'
    return
  }
  if (needsAdmin.value && password.value.length < 8) {
    errorMessage.value = '管理员密码至少需要 8 位。'
    return
  }

  busy.value = true
  errorMessage.value = ''
  try {
    if (needsAdmin.value) {
      const result = await api.post('/auth/bootstrap/register', {
        username: account,
        password: password.value,
      }, { auth: false })
      if (Number(result?.code) !== 0) throw new Error(result?.msg || '管理员创建失败')
    } else {
      const body = new URLSearchParams({ username: account, password: password.value }).toString()
      const result = await api.post('/auth/login', body, {
        auth: false,
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      })
      if (Number(result?.errcode) !== 200) throw new Error(result?.errmsg || copy.loginFailed)
    }
    window.location.assign('/IntelligentAI')
  } catch (error) {
    errorMessage.value = error?.message || (needsAdmin.value ? '创建管理员失败，请重试。' : copy.loginFailed)
    busy.value = false
    if (needsAdmin.value) await loadBootstrapStatus()
  }
}

onMounted(loadBootstrapStatus)
</script>

<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <BrandMark class="login-brand-mark" />
      <p class="login-eyebrow">{{ copy.appName }}</p>
      <h1 id="login-title">{{ needsAdmin ? '创建平台管理员' : copy.loginTitle }}</h1>
      <p class="login-subtitle">
        {{ needsAdmin ? '平台已就绪，请创建第一个管理员账户。此账户将拥有平台管理权限。' : copy.loginSubtitle }}
      </p>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
      <p v-if="loading" class="login-subtitle">{{ copy.loading }}</p>

      <div v-if="!loading && !needsAdmin && localLoginEnabled && ssoLoginEnabled && !loginChoice" class="login-method-choices">
        <button class="login-method-card" type="button" @click="chooseLogin('local')">
          <span class="login-method-icon">账号</span>
          <span><strong>平台账户登录</strong><small>使用管理员分配的账号和密码</small></span>
          <span class="login-method-arrow">›</span>
        </button>
        <button class="login-method-card" type="button" @click="chooseLogin('sso')">
          <span class="login-method-icon sso">SSO</span>
          <span><strong>学校统一身份认证</strong><small>使用学校统一身份认证账号登录</small></span>
          <span class="login-method-arrow">›</span>
        </button>
      </div>

      <form v-else-if="needsAdmin || (localLoginEnabled && (!ssoLoginEnabled || loginChoice === 'local'))" class="login-form" @submit="submit">
        <label class="field-label" for="username">{{ copy.username }}</label>
        <input id="username" v-model="username" name="username" class="login-input"
          autocomplete="username" :placeholder="copy.usernamePlaceholder" maxlength="50" required />

        <label class="field-label" for="password">{{ copy.password }}</label>
        <input id="password" v-model="password" name="password" class="login-input" type="password"
          :autocomplete="needsAdmin ? 'new-password' : 'current-password'"
          :placeholder="needsAdmin ? '至少 8 位' : copy.passwordPlaceholder" minlength="8" maxlength="72" required />

        <template v-if="needsAdmin">
          <label class="field-label" for="confirm-password">确认密码</label>
          <input id="confirm-password" v-model="confirmPassword" class="login-input" type="password"
            autocomplete="new-password" placeholder="再次输入密码" minlength="8" maxlength="72" required />
        </template>

        <button class="login-submit" type="submit" :disabled="busy">
          <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
          {{ busy ? (needsAdmin ? '正在创建…' : copy.loggingIn) : (needsAdmin ? '创建管理员并进入平台' : copy.login) }}
        </button>
      </form>

      <div v-if="!loading && !needsAdmin && ssoLoginEnabled && (!localLoginEnabled || loginChoice === 'sso')" class="login-sso-actions">
        <p>使用学校统一身份认证登录，认证成功后将自动进入对话。</p>
        <button class="login-submit" type="button" @click="startSSO">前往学校统一身份认证</button>
        <button v-if="localLoginEnabled" class="login-method-back" type="button" @click="backToLoginChoices">返回选择登录方式</button>
      </div>
      <button v-if="!loading && !needsAdmin && localLoginEnabled && ssoLoginEnabled && loginChoice === 'local'" class="login-method-back" type="button" @click="backToLoginChoices">返回选择登录方式</button>

      <div class="agreement-row">
        <span>{{ needsAdmin ? '首个注册账户自动成为管理员；后续账户由管理员创建。' : (localLoginEnabled && ssoLoginEnabled ? '可在首页选择平台账户登录或学校统一身份认证。' : (ssoLoginEnabled ? '登录状态由学校统一身份认证平台管理。' : '账号由平台管理员创建和管理。')) }}</span>
      </div>
    </section>
    <p class="login-footer">{{ copy.appName }}</p>
  </main>
</template>
