import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'

function HealthDot({ status, latency }) {
  const colors = { up: '#3fb950', down: '#f85149', unknown: '#6e7681', maintenance: '#d29922' }
  const color = colors[status] || colors.unknown
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:5}}>
      <span style={{
        width:8, height:8, borderRadius:'50%', background:color,
        display:'inline-block', flexShrink:0,
        boxShadow: status === 'up' ? `0 0 4px ${color}` : 'none'
      }}/>
      {latency > 0 && <span style={{fontSize:11, color:'var(--text3)'}}>{latency}ms</span>}
    </span>
  )
}

function CertBadge({ certStatus, certExpiry }) {
  if (!certStatus || certStatus === 'none') {
    return <span className="badge badge-gray">no cert</span>
  }
  if (certStatus === 'active' && certExpiry) {
    const days = Math.floor((new Date(certExpiry * 1000) - Date.now()) / 86400000)
    if (days < 14) return <span className="badge badge-red">expires {days}d</span>
    if (days < 30) return <span className="badge badge-yellow">{days}d left</span>
    return <span className="badge badge-green">valid</span>
  }
  if (certStatus === 'pending') return <span className="badge badge-yellow">pending</span>
  if (certStatus === 'expired') return <span className="badge badge-red">expired</span>
  return <span className="badge badge-gray">none</span>
}

export function DashboardPage() {
  const [hosts, setHosts] = useState([])
  const [dests, setDests] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([api.listHosts(), api.listDestinations()])
      .then(([h, d]) => { setHosts(h); setDests(d) })
      .finally(() => setLoading(false))
  }, [])

  const enabledHosts = hosts.filter(h => h.Enabled)
  const upHosts = hosts.filter(h => h.HealthStatus === 'up').length
  const downHosts = hosts.filter(h => h.HealthStatus === 'down').length
  const activeCerts = hosts.filter(h => h.CertStatus === 'active').length
  const pendingCerts = hosts.filter(h => h.Enabled && h.CertStatus === 'pending').length

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      <div className="page-header">
        <h1>Dashboard</h1>
        <p>Overview of your proxy configuration</p>
      </div>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-label">Hosts up</div>
          <div className={`stat-value ${downHosts > 0 ? 'red' : 'green'}`}>{upHosts}</div>
          <div className="text-muted text-sm mt-1">of {enabledHosts.length} enabled</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Hosts down</div>
          <div className={`stat-value ${downHosts > 0 ? 'red' : 'green'}`}>{downHosts}</div>
          <div className="text-muted text-sm mt-1">
            {hosts.filter(h => h.HealthStatus === 'unknown').length} unchecked
          </div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Valid certs</div>
          <div className={`stat-value ${pendingCerts > 0 ? 'yellow' : 'green'}`}>{activeCerts}</div>
          <div className="text-muted text-sm mt-1">{pendingCerts > 0 ? `${pendingCerts} pending` : 'all good'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Metric destinations</div>
          <div className="stat-value blue">{dests.filter(d => d.Enabled).length}</div>
          <div className="text-muted text-sm mt-1">{dests.length} configured</div>
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <span className="card-title">Proxy hosts</span>
          <Link to="/hosts" className="btn btn-sm">Manage</Link>
        </div>
        {hosts.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon">🌐</div>
            <h3>No proxy hosts yet</h3>
            <p>Add your first proxy host to get started.</p>
            <Link to="/hosts" className="btn btn-primary">Add host</Link>
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Health</th>
                  <th>Domain</th>
                  <th>Upstream</th>
                  <th>SSL</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {hosts.map(h => (
                  <tr key={h.ID}>
                    <td><HealthDot status={h.HealthStatus} latency={h.HealthLatencyMs}/></td>
                    <td><code>{h.Domain}</code></td>
                    <td><span className="text-muted text-sm">{h.Upstream}</span></td>
                    <td><CertBadge certStatus={h.CertStatus} certExpiry={h.CertExpiry}/></td>
                    <td>
                      <span className={`badge ${h.Enabled ? 'badge-green' : 'badge-gray'}`}>
                        {h.Enabled ? 'active' : 'disabled'}
                      </span>
                    </td>
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
