const SIGNED_IN_COOKIE = 'pingtai_signed_in'

function readCookie(name) {
  const prefix = `${name}=`
  const item = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix))
  return item ? decodeURIComponent(item.slice(prefix.length)) : ''
}

// This flag only controls SPA navigation. Authentication is enforced by the
// HttpOnly Authorization cookie and verified by the Go API on every request.
export function hasAccessToken() {
  return readCookie(SIGNED_IN_COOKIE) === '1'
}

export function clearAuth() {
  document.cookie = `${SIGNED_IN_COOKIE}=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax`
  localStorage.removeItem('username')
}
