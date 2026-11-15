const ACCESS_COOKIE = 'Authorization'

function readCookie(name) {
  const prefix = `${name}=`
  const item = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix))
  return item ? decodeURIComponent(item.slice(prefix.length)) : ''
}

export function getAccessToken() {
  return readCookie(ACCESS_COOKIE)
}

export function setAccessToken(token, expiresAt) {
  const expiry = expiresAt
    ? new Date(Number(expiresAt) * 1000)
    : new Date(Date.now() + 30 * 60 * 1000)
  const secure = window.location.protocol === 'https:' ? '; Secure' : ''
  document.cookie = `${ACCESS_COOKIE}=${encodeURIComponent(token)}; Path=/; Expires=${expiry.toUTCString()}; SameSite=Lax${secure}`
}

export function clearAuth() {
  document.cookie = `${ACCESS_COOKIE}=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax`
  localStorage.removeItem('refresh_token')
  localStorage.removeItem('username')
}

export function hasAccessToken() {
  return Boolean(getAccessToken())
}
