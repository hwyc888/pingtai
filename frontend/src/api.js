import { clearAuth } from './auth'

const baseURL = (window.g?.baseURL || import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
let refreshPromise = null

function buildURL(path, params) {
  const url = new URL(`${baseURL}${path}`, window.location.origin)
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null) url.searchParams.set(key, String(value))
    }
  }
  return url
}

async function readResponse(response) {
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return { msg: text }
  }
}

async function refreshAccessToken() {
  if (refreshPromise) return refreshPromise
  refreshPromise = (async () => {
    const response = await fetch(buildURL('/auth/refresh'), {
      method: 'POST',
      credentials: 'same-origin',
    })
    const data = await readResponse(response)
    return response.ok && data?.errcode === 200
  })().finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

export async function request(path, options = {}) {
  const {
    method = 'GET',
    params,
    body,
    auth = true,
    retry = true,
    headers: customHeaders = {},
  } = options
  const headers = new Headers(customHeaders)
  let payload = body
  if (payload !== undefined && payload !== null && !(payload instanceof FormData) && typeof payload !== 'string') {
    headers.set('Content-Type', 'application/json')
    payload = JSON.stringify(payload)
  }

  const response = await fetch(buildURL(path, params), {
    method,
    headers,
    body: method === 'GET' || method === 'HEAD' ? undefined : payload,
    credentials: 'same-origin',
  })

  if (response.status === 401 && auth && retry) {
    const refreshed = await refreshAccessToken().catch(() => false)
    if (refreshed) return request(path, { ...options, retry: false })
    clearAuth()
    if (window.location.pathname !== '/login') window.location.assign('/login')
  }

  const data = await readResponse(response)
  if (!response.ok) {
    const message = data?.error?.message || data?.msg || `HTTP ${response.status}`
    throw new Error(message)
  }
  return data
}

export const api = {
  get(path, params, options = {}) {
    return request(path, { ...options, method: 'GET', params })
  },
  post(path, body, options = {}) {
    return request(path, { ...options, method: 'POST', body })
  },
}
