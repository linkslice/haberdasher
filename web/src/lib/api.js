const BASE = ''

function getToken() {
  return localStorage.getItem('haber_token')
}

export function setToken(tok) {
  localStorage.setItem('haber_token', tok)
}

export function clearToken() {
  localStorage.removeItem('haber_token')
}

export function isAuthenticated() {
  return !!getToken()
}

async function request(method, path, body) {
  const headers = { 'Content-Type': 'application/json' }
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok

  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new Error('unauthorized')
  }

  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data
}

export const api = {
  // Setup
  setupStatus: () => request('GET', '/api/setup/status'),
  setupComplete: (email, password) => request('POST', '/api/setup/complete', { email, password }),

  // Auth
  login: (email, password, totp_code) => request('POST', '/api/auth/login', { email, password, totp_code }),
  me: () => request('GET', '/api/auth/me'),
  changePassword: (current_password, new_password) => request('POST', '/api/auth/change-password', { current_password, new_password }),
  totpSetup: () => request('POST', '/api/auth/totp/setup'),
  totpConfirm: (code) => request('POST', '/api/auth/totp/confirm', { code }),
  totpDisable: () => request('DELETE', '/api/auth/totp'),

  // Hosts
  listHosts: () => request('GET', '/api/hosts'),
  createHost: (data) => request('POST', '/api/hosts', data),
  updateHost: (id, data) => request('PUT', `/api/hosts/${id}`, data),
  deleteHost: (id) => request('DELETE', `/api/hosts/${id}`),
  toggleHost: (id) => request('POST', `/api/hosts/${id}/toggle`),
  toggleMaintenance: (id) => request('POST', `/api/hosts/${id}/maintenance`),

  // Rules
  listRules: (hostId) => request('GET', `/api/hosts/${hostId}/rules`),
  createRule: (hostId, data) => request('POST', `/api/hosts/${hostId}/rules`, data),
  deleteRule: (hostId, ruleId) => request('DELETE', `/api/hosts/${hostId}/rules/${ruleId}`),

  // Certs
  listCerts: () => request('GET', '/api/certificates'),
  requestCert: (hostId, data) => request('POST', `/api/hosts/${hostId}/certificate`, data),
  getCert: (hostId) => request('GET', `/api/hosts/${hostId}/certificate`),
  deleteCert: (certId) => request('DELETE', `/api/certificates/${certId}`),

  // Monitoring
  listDestinations: () => request('GET', '/api/monitoring/destinations'),
  createDestination: (data) => request('POST', '/api/monitoring/destinations', data),
  updateDestination: (id, data) => request('PUT', `/api/monitoring/destinations/${id}`, data),
  deleteDestination: (id) => request('DELETE', `/api/monitoring/destinations/${id}`),
  testDestination: (id) => request('POST', `/api/monitoring/destinations/${id}/test`),
  toggleDestination: (id) => request('POST', `/api/monitoring/destinations/${id}/toggle`),

  // Settings
  getSettings: () => request('GET', '/api/settings'),
  updateSettings: (data) => request('PUT', '/api/settings', data),

  // Audit
  auditLog: () => request('GET', '/api/audit'),
}
