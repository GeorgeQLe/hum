const BASE = '/api'

async function request(path: string, options?: RequestInit) {
  const resp = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  return resp.json()
}

export const api = {
  // Auth
  login: (email: string, password: string) =>
    request('/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),

  register: (email: string, password: string) =>
    request('/auth/register', { method: 'POST', body: JSON.stringify({ email, password }) }),

  // Environments
  getEnvironments: () => request('/environments'),

  createEnvironment: (name: string) =>
    request('/environments', { method: 'POST', body: JSON.stringify({ name }) }),

  // Secrets
  listSecrets: (env: string) => request(`/secrets/${env}`),

  getSecret: (env: string, key: string) => request(`/secrets/${env}/${key}`),

  setSecret: (env: string, key: string, value: string) =>
    request(`/secrets/${env}/${key}`, { method: 'PUT', body: JSON.stringify({ value }) }),

  deleteSecret: (env: string, key: string) =>
    request(`/secrets/${env}/${key}`, { method: 'DELETE' }),

  // Users
  listUsers: () => request('/users'),

  setUserRole: (id: string, role: string) =>
    request(`/users/${id}/role`, { method: 'PUT', body: JSON.stringify({ role }) }),

  // Audit
  getAuditLog: () => request('/audit'),

  // Share
  createShare: (environment: string, key: string, expiresIn: number) =>
    request('/share', { method: 'POST', body: JSON.stringify({ environment, key, expires_in: expiresIn }) }),
}
