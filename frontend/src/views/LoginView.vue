<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { setAccessToken } from '../auth'
import { copy } from '../copy'
import loginLogo from '../../assets/login-9prbKjc7.png'

const router = useRouter()
const username = ref('')
const password = ref('')
const accepted = ref(false)
const busy = ref(false)
const errorMessage = ref('')

async function submit() {
  errorMessage.value = ''
  if (!accepted.value) {
    errorMessage.value = copy.agreementRequired
    return
  }
  if (busy.value) return

  busy.value = true
  try {
    const form = new FormData()
    form.append('username', username.value.trim())
    form.append('password', password.value)
    const result = await api.post('/auth/login', form, { auth: false })
    if (Number(result?.errcode) !== 200 || !result?.access_token) {
      throw new Error(result?.errmsg || copy.requestFailed)
    }
    localStorage.setItem('refresh_token', result.refresh_token || '')
    localStorage.setItem('username', username.value.trim())
    setAccessToken(result.access_token, result.token_expiration)
    await router.replace('/IntelligentAI')
  } catch (error) {
    errorMessage.value = error?.message || copy.requestFailed
  } finally {
    busy.value = false
  }
}
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
      <p class="login-subtitle">{{ copy.loginSubtitle }}</p>

      <form class="login-form" @submit.prevent="submit">
        <label class="field-label" for="username">{{ copy.username }}</label>
        <input
          id="username"
          v-model="username"
          class="login-input"
          type="text"
          name="username"
          autocomplete="username"
          :placeholder="copy.usernamePlaceholder"
          required
        />

        <label class="field-label" for="password">{{ copy.password }}</label>
        <input
          id="password"
          v-model="password"
          class="login-input"
          type="password"
          name="password"
          autocomplete="current-password"
          :placeholder="copy.passwordPlaceholder"
          required
        />

        <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

        <button class="login-submit" type="submit" :disabled="busy">
          <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
          {{ busy ? copy.loggingIn : copy.login }}
        </button>
      </form>

      <div class="agreement-row">
        <input id="agreement" v-model="accepted" type="checkbox" />
        <label for="agreement">
          {{ copy.agreement }}
          <a href="#terms" @click.prevent>{{ copy.terms }}</a>
          {{ copy.and }}
          <a href="#privacy" @click.prevent>{{ copy.privacy }}</a>
        </label>
      </div>
    </section>
    <p class="login-footer">{{ copy.appName }}</p>
  </main>
</template>
