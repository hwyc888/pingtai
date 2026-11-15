<script setup>
import { onMounted, ref } from 'vue'
import { copy } from '../copy'
import loginLogo from '../../assets/login-9prbKjc7.png'

const busy = ref(false)
const errorMessage = ref('')

function login() {
  if (busy.value) return
  busy.value = true
  window.location.assign('/auth/sso/login')
}

onMounted(() => {
  const params = new URLSearchParams(window.location.search)
  if (params.get('sso_error') === '1') {
    errorMessage.value = copy.ssoLoginError
    window.history.replaceState(null, '', window.location.pathname)
  }
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
      <p class="login-subtitle">{{ copy.ssoLoginHint }}</p>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

      <button class="login-submit" type="button" :disabled="busy" @click="login">
        <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
        {{ busy ? copy.loggingIn : copy.ssoLogin }}
      </button>

      <div class="agreement-row">
        <span>{{ copy.ssoLoginFooter }}</span>
      </div>
    </section>
    <p class="login-footer">{{ copy.appName }}</p>
  </main>
</template>
