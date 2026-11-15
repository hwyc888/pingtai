<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const loading = ref(true)
const saving = ref(false)
const mode = ref('local')
const users = ref([])
const total = ref(0)
const errorMessage = ref('')
const successMessage = ref('')
const username = ref('')
const password = ref('')
const userType = ref(0)
const resetUserId = ref(0)
const resetPassword = ref('')

function assertAdminResponse(result) {
  if (Number(result?.errcode) !== 200) throw new Error(result?.errmsg || '请求失败')
  return result
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
    if (mode.value === 'local') await loadUsers()
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
  if (!window.confirm(`确定删除账号“${user.username}”吗？`)) return
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
        <p class="admin-eyebrow">PINGTAI ADMIN</p>
        <h1>平台管理</h1>
        <p class="admin-subtitle">管理平台账号、认证方式和 AI 服务配置</p>
      </div>
      <div class="admin-header-actions">
        <button class="admin-secondary" type="button" @click="router.push('/IntelligentAI')">返回对话</button>
        <button class="admin-primary" type="button" @click="router.push('/setup')">供应商与模型配置</button>
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
            <p>当前由平台配置决定登录方式，普通用户无需自行选择。</p>
          </div>
          <span class="admin-mode">{{ mode === 'sso' ? '学校统一身份认证' : '本地账号密码' }}</span>
        </div>
        <p v-if="mode === 'sso'" class="admin-note">当前使用学校 SSO。管理员权限由 SSO 管理员账号白名单控制；本地账号管理暂不适用于此模式。</p>
        <p v-else class="admin-note">当前使用本地账号。请在下方创建和管理用户账号。</p>
      </section>

      <section v-if="mode === 'local'" class="admin-card">
        <div class="admin-card-heading">
          <div>
            <h2>添加用户</h2>
            <p>账号由管理员创建，用户登录后直接进入 AI 对话。</p>
          </div>
        </div>
        <form class="admin-form" @submit.prevent="addUser">
          <label>账号<input v-model="username" maxlength="50" autocomplete="off" placeholder="输入用户名" required /></label>
          <label>初始密码<input v-model="password" type="password" minlength="8" maxlength="72" autocomplete="new-password" placeholder="至少 8 位" required /></label>
          <label>角色<select v-model.number="userType"><option :value="0">普通用户</option><option :value="1">管理员</option></select></label>
          <button class="admin-primary" type="submit" :disabled="saving">创建账号</button>
        </form>
      </section>

      <section v-if="mode === 'local'" class="admin-card">
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
  </main>
</template>
