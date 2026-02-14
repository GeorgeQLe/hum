import React, { useEffect, useState } from 'react'
import { api } from './api'

type View = 'secrets' | 'environments' | 'users' | 'audit'

interface Environment {
  name: string
}

export default function App() {
  const [view, setView] = useState<View>('secrets')
  const [environments, setEnvironments] = useState<string[]>([])
  const [selectedEnv, setSelectedEnv] = useState('development')
  const [secrets, setSecrets] = useState<string[]>([])
  const [newKey, setNewKey] = useState('')
  const [newValue, setNewValue] = useState('')

  useEffect(() => {
    loadEnvironments()
  }, [])

  useEffect(() => {
    if (selectedEnv) loadSecrets(selectedEnv)
  }, [selectedEnv])

  async function loadEnvironments() {
    const data = await api.getEnvironments()
    setEnvironments(data.environments || [])
  }

  async function loadSecrets(env: string) {
    const data = await api.listSecrets(env)
    setSecrets(data.keys || [])
  }

  async function handleSetSecret(e: React.FormEvent) {
    e.preventDefault()
    if (!newKey || !newValue) return
    await api.setSecret(selectedEnv, newKey, newValue)
    setNewKey('')
    setNewValue('')
    loadSecrets(selectedEnv)
  }

  async function handleDeleteSecret(key: string) {
    await api.deleteSecret(selectedEnv, key)
    loadSecrets(selectedEnv)
  }

  return (
    <div style={{ maxWidth: 900, margin: '0 auto', padding: 20 }}>
      <header style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, color: '#58a6ff' }}>envsafe</h1>
        <nav style={{ display: 'flex', gap: 8 }}>
          {(['secrets', 'environments', 'users', 'audit'] as View[]).map(v => (
            <button
              key={v}
              onClick={() => setView(v)}
              style={{
                padding: '6px 12px',
                background: view === v ? '#1f6feb' : '#21262d',
                color: '#c9d1d9',
                border: '1px solid #30363d',
                borderRadius: 6,
                cursor: 'pointer',
              }}
            >
              {v}
            </button>
          ))}
        </nav>
      </header>

      {view === 'secrets' && (
        <div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ marginRight: 8 }}>Environment:</label>
            <select
              value={selectedEnv}
              onChange={e => setSelectedEnv(e.target.value)}
              style={{ padding: '4px 8px', background: '#21262d', color: '#c9d1d9', border: '1px solid #30363d', borderRadius: 4 }}
            >
              {environments.map(env => (
                <option key={env} value={env}>{env}</option>
              ))}
            </select>
          </div>

          <form onSubmit={handleSetSecret} style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            <input
              placeholder="KEY"
              value={newKey}
              onChange={e => setNewKey(e.target.value)}
              style={{ padding: '6px 8px', background: '#0d1117', color: '#c9d1d9', border: '1px solid #30363d', borderRadius: 4, flex: 1 }}
            />
            <input
              placeholder="value"
              type="password"
              value={newValue}
              onChange={e => setNewValue(e.target.value)}
              style={{ padding: '6px 8px', background: '#0d1117', color: '#c9d1d9', border: '1px solid #30363d', borderRadius: 4, flex: 2 }}
            />
            <button type="submit" style={{ padding: '6px 16px', background: '#238636', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer' }}>
              Set
            </button>
          </form>

          <div>
            {secrets.length === 0 ? (
              <p style={{ color: '#8b949e' }}>No secrets in this environment.</p>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid #30363d' }}>
                    <th style={{ textAlign: 'left', padding: 8 }}>Key</th>
                    <th style={{ textAlign: 'left', padding: 8 }}>Value</th>
                    <th style={{ padding: 8, width: 80 }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {secrets.map(key => (
                    <tr key={key} style={{ borderBottom: '1px solid #21262d' }}>
                      <td style={{ padding: 8, fontFamily: 'monospace', color: '#7ee787' }}>{key}</td>
                      <td style={{ padding: 8, color: '#8b949e' }}>••••••••</td>
                      <td style={{ padding: 8, textAlign: 'center' }}>
                        <button
                          onClick={() => handleDeleteSecret(key)}
                          style={{ padding: '2px 8px', background: '#da3633', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12 }}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}

      {view === 'environments' && (
        <div>
          <h2 style={{ marginBottom: 16 }}>Environments</h2>
          <ul style={{ listStyle: 'none' }}>
            {environments.map(env => (
              <li key={env} style={{ padding: '8px 0', borderBottom: '1px solid #21262d' }}>{env}</li>
            ))}
          </ul>
        </div>
      )}

      {view === 'users' && (
        <div>
          <h2 style={{ marginBottom: 16 }}>Users</h2>
          <p style={{ color: '#8b949e' }}>User management requires server authentication.</p>
        </div>
      )}

      {view === 'audit' && (
        <div>
          <h2 style={{ marginBottom: 16 }}>Audit Log</h2>
          <p style={{ color: '#8b949e' }}>Audit log requires admin access.</p>
        </div>
      )}
    </div>
  )
}
