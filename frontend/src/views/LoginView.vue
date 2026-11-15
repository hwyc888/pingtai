<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { copy } from '../copy'
import loginLogo from '../../assets/login-9prbKjc7.png'

const busy = ref(false)
const loading = ref(true)
const mode = ref('local')
const errorMessage = ref('')

async function loadMode() {
  try {
    const result = await api.get('/auth/mode', undefined, { auth: false })
    mode.value = result?.data?.mode === 'sso' ? 'sso' : 'local'
  } catch {
    errorMessage.value = copy.loginModeUnavailable
  } finally {
    loading.value = false
  }
}

async function loginLocal(event) {
  event.preventDefault()
  if (busy.value) return
  const form = new FormData(event.currentTarget)
  const username = String(form.get('username') || '').trim()
  const password = String(form.get('password') || '')
  if (!username || !password) {
    errorMessage.value = copy.credentialsRequired
    return
  }
  busy.value = true
  errorMessage.value = ''
  try {
    const body = new URLSearchParams({ username, password }).toString()
    const result = await api.post('/auth/login', body, {
      auth: false,
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    })
    if (Number(result?.errcode) !== 200) throw new Error(result?.errmsg || copy.loginFailed)
    window.location.assign('/IntelligentAI')
  } catch (error) {
    errorMessage.value = error?.message || copy.loginFailed
    busy.value = false
  }
}

function loginSSO() {
  if (busy.value) return
  busy.value = true
  window.location.assign('/auth/sso/login')
}

onMounted(async () => {
  const params = new URLSearchParams(window.location.search)
  if (params.get('sso_error') === '1') {
    errorMessage.value = copy.ssoLoginError
    window.history.replaceState(null, '', window.location.pathname)
  }
  await loadMode()
})
</script>

<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <div class="login-brand-mark">
        <img :src="loginLogo" alt="AI" />
        <i></i>
      </div>
      <p class="login-eyebrow">{{ copy.appName }}</p>
      <h1 id="login-title">{{ copy.loginTitle }}</h1>
      <p class="login-subtitle">{{ mode === 'sso' ? copy.ssoLoginHint : copy.loginSubtitle }}</p>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
      <p v-if="loading" class="login-subtitle">{{ copy.loading }}</p>

      <form v-else-if="mode === 'local'" class="login-form" @submit="loginLocal">
        <label class="field-label" for="username">{{ copy.username }}</label>
        <input id="username" name="username" class="login-input" autocomplete="username" :placeholder="copy.usernamePlaceholder" required />
        <label class="field-label" for="password">{{ copy.password }}</label>
        <input id="password" name="password" class="login-input" type="password" autocomplete="current-password" :placeholder="copy.passwordPlaceholder" required />
        <button class="login-submit" type="submit" :disabled="busy">
          <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
          {{ busy ? copy.loggingIn : copy.login }}
        </button>
      </form>

      <button v-else class="login-submit" type="button" :disabled="busy" @click="loginSSO">
        <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
        {{ busy ? copy.loggingIn : copy.ssoLogin }}
      </button>

      <div class="agreement-row">
        <span>{{ mode === 'sso' ? copy.ssoLoginFooter : copy.localLoginFooter }}</span>
      </div>
    </section>
    <p class="login-footer">{{ copy.appName }}</p>
  </main>
</template>
