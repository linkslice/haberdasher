import { useState, useEffect } from 'react'
import { api } from '../lib/api'

const ACTION_COLORS = {
  host_created: 'badge-green', host_updated: 'badge-blue', host_deleted: 'badge-red',
  host_toggled: 'badge-blue', rule_created: 'badge-green', rule_deleted: 'badge-red',
  cert_requested: 'badge-yellow', metric_dest_created: 'badge-green',
  metric_dest_updated: 'badge-blue', metric_dest_deleted: 'badge-red',
  metric_dest_toggled: 'badge-blue', password_changed: 'badge-yellow',
  totp_enabled: 'badge-green', totp_disabled: 'badge-red', settings_updated: 'badge-blue',
}

export function AuditLogPage() {
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.auditLog().then(setEntries).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      <div className="page-header">
        <h1>Audit log</h1>
        <p>Last 200 administrative actions</p>
      </div>

      <div className="card">
        {entries.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon">📋</div>
            <h3>No audit entries yet</h3>
            <p>Actions will appear here as you configure Haberdasher.</p>
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>Time</th><th>Action</th><th>Target</th><th>Detail</th></tr>
              </thead>
              <tbody>
                {entries.map(e => (
                  <tr key={e.id}>
                    <td style={{whiteSpace:'nowrap'}}>
                      <span className="text-muted text-sm">
                        {new Date(e.created_at * 1000).toLocaleString()}
                      </span>
                    </td>
                    <td>
                      <span className={`badge ${ACTION_COLORS[e.action] || 'badge-gray'}`}>
                        {e.action.replace(/_/g, ' ')}
                      </span>
                    </td>
                    <td><code style={{fontSize:12}}>{e.target}</code></td>
                    <td><span className="text-muted text-sm">{e.detail || '—'}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}
