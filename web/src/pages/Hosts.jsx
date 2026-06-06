import { useState, useEffect } from 'react'
import { api } from '../lib/api'
import { Modal } from '../components/Modal'
import { Toggle } from '../components/Toggle'
import { Plus, Pencil, Trash2, ShieldCheck, Lock, WrenchIcon } from 'lucide-react'

function HealthDot({ status, label, latency }) {
  const colors = { up: '#3fb950', down: '#f85149', unknown: '#6e7681', maintenance: '#d29922' }
  const color = colors[status] || colors.unknown
  return (
    <span title={`${label}: ${status}${latency > 0 ? ` (${latency}ms)` : ''}`}
      style={{display:'inline-flex', alignItems:'center', gap:4}}>
      <span style={{
        width:8, height:8, borderRadius:'50%', background:color,
        display:'inline-block', flexShrink:0,
        boxShadow: status === 'up' ? `0 0 4px ${color}` : 'none'
      }}/>
      <span style={{fontSize:11, color:'var(--text3)'}}>{label}</span>
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

function HostForm({ initial, onSave, onCancel }) {
  const [form, setForm] = useState(() => {
    if (!initial) return {
      domain: '', upstream: '', metrics_alias: '',
      strip_path: false, websocket: true, force_https: true
    }
    return {
      domain:        initial.Domain        || '',
      upstream:      initial.Upstream      || '',
      metrics_alias: initial.MetricsAlias  || '',
      strip_path:    initial.StripPath     || false,
      websocket:     initial.WebSocket     !== undefined ? initial.WebSocket : true,
      force_https:   initial.ForceHTTPS    !== undefined ? initial.ForceHTTPS : true,
    }
  })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  function set(k, v) { setForm(f => ({...f, [k]: v})) }

  async function submit(e) {
    e.preventDefault()
    setError('')
    setSaving(true)
    try { await onSave(form) }
    catch (err) { setError(err.message) }
    finally { setSaving(false) }
  }

  return (
    <form onSubmit={submit}>
      {error && <div className="alert alert-error">{error}</div>}
      <div className="form-group">
        <label className="form-label">Domain</label>
        <input className="form-input" placeholder="app.example.com" value={form.domain}
          onChange={e => set('domain', e.target.value)} required/>
        <p className="form-hint">The public domain this proxy will respond to</p>
      </div>
      <div className="form-group">
        <label className="form-label">Upstream</label>
        <input className="form-input" placeholder="localhost:3000 or http://192.168.1.5:8080"
          value={form.upstream} onChange={e => set('upstream', e.target.value)} required/>
        <p className="form-hint">Where traffic gets forwarded to</p>
      </div>
      <div className="form-group">
        <label className="form-label">Metrics alias <span style={{color:'var(--text3)'}}>optional</span></label>
        <input className="form-input" placeholder="my-app"
          value={form.metrics_alias} onChange={e => set('metrics_alias', e.target.value)}/>
      </div>
      <hr className="divider"/>
      <div style={{display:'flex', flexDirection:'column', gap:12}}>
        <label className="form-check">
          <input type="checkbox" checked={form.force_https} onChange={e => set('force_https', e.target.checked)}/>
          Force HTTPS redirect
        </label>
        <label className="form-check">
          <input type="checkbox" checked={form.websocket} onChange={e => set('websocket', e.target.checked)}/>
          WebSocket support
        </label>
        <label className="form-check">
          <input type="checkbox" checked={form.strip_path} onChange={e => set('strip_path', e.target.checked)}/>
          Strip path prefix
        </label>
      </div>
      <div className="modal-footer" style={{padding:'16px 0 0', borderTop:'none'}}>
        <button type="button" className="btn" onClick={onCancel}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Saving…' : 'Save host'}
        </button>
      </div>
    </form>
  )
}

function CertModal({ host, onClose, onDeleted }) {
  const [cert, setCert] = useState(null)
  const [provider, setProvider] = useState('letsencrypt')
  const [form, setForm] = useState({
    acme_email: '', dns_provider: '', dns_config: '',
    cert_pem: '', key_pem: '', valid_days: 365
  })
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    api.getCert(host.ID).then(c => { if (c) setCert(c) }).catch(() => {})
  }, [host.ID])

  function set(k, v) { setForm(f => ({...f, [k]: v})) }

  async function submit(e) {
    e.preventDefault()
    setError(''); setSaving(true)
    try {
      const payload = { provider, ...form }
      const c = await api.requestCert(host.ID, payload)
      setCert(c)
      setSuccess(provider === 'letsencrypt'
        ? 'Certificate request submitted. Caddy will obtain it automatically via ACME.'
        : 'Certificate saved successfully.')
    } catch(err) { setError(err.message) }
    finally { setSaving(false) }
  }

  async function handleDelete() {
    if (!confirm('Delete this certificate? The host will show no cert until you request a new one.')) return
    setDeleting(true)
    try {
      await api.deleteCert(cert.ID)
      onDeleted && onDeleted()
      onClose()
    } catch(err) { setError(err.message) }
    finally { setDeleting(false) }
  }

  const daysLeft = cert?.Expiry ? Math.floor((new Date(cert.Expiry * 1000) - Date.now()) / 86400000) : null

  const providerBadge = cert ? ({
    letsencrypt: cert.Staging
      ? <span className="badge badge-yellow">Let's Encrypt (staging)</span>
      : <span className="badge badge-blue">Let's Encrypt</span>,
    selfsigned:  <span className="badge badge-yellow">self-signed</span>,
    custom:      <span className="badge badge-blue">custom</span>,
  }[cert.Provider] || <span className="badge badge-gray">{cert.Provider}</span>) : null

  return (
    <Modal title={`SSL — ${host.Domain}`} onClose={onClose}>
      {cert ? (
        <div>
          {success && <div className="alert alert-success">{success}</div>}
          {error && <div className="alert alert-error">{error}</div>}
          <div style={{display:'grid', gridTemplateColumns:'1fr 1fr', gap:12, marginBottom:16}}>
            <div className="stat-card">
              <div className="stat-label">Provider</div>
              <div style={{marginTop:4}}>{providerBadge}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">Status</div>
              <span className={`badge ${cert.Status === 'active' ? (cert.Provider === 'selfsigned' ? 'badge-yellow' : 'badge-green') : 'badge-yellow'}`}>
                {cert.Status}
              </span>
            </div>
          </div>
          {daysLeft !== null && (
            <div className="stat-card" style={{marginBottom:16}}>
              <div className="stat-label">Expiry</div>
              <div className={`stat-value ${daysLeft < 14 ? 'red' : daysLeft < 30 ? 'yellow' : 'green'}`} style={{fontSize:18}}>
                {daysLeft > 0 ? `${daysLeft} days` : 'Expired'}
              </div>
              <div className="text-muted text-sm mt-1">{new Date(cert.Expiry * 1000).toLocaleDateString()}</div>
            </div>
          )}
          {cert.ACMEEmail && (
            <div className="form-group">
              <label className="form-label">ACME email</label>
              <input className="form-input" value={cert.ACMEEmail} readOnly/>
            </div>
          )}
          {cert.Provider === 'selfsigned' && (
            <div className="alert alert-info" style={{marginBottom:16}}>
              Self-signed certificates are not trusted by browsers. Use for internal/testing only.
            </div>
          )}
          <div style={{display:'flex', gap:8, justifyContent:'flex-end'}}>
            <button className="btn btn-danger btn-sm" onClick={handleDelete} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete certificate'}
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit}>
          {error && <div className="alert alert-error">{error}</div>}
          <div className="form-group">
            <label className="form-label">Certificate type</label>
            <select className="form-select" value={provider} onChange={e => setProvider(e.target.value)}>
              <option value="letsencrypt">Let's Encrypt (ACME)</option>
              <option value="selfsigned">Self-signed</option>
              <option value="custom">Upload existing certificate</option>
            </select>
          </div>

          {provider === 'letsencrypt' && (
            <>
              <div className="form-group">
                <label className="form-label">ACME email</label>
                <input className="form-input" type="email" placeholder="you@example.com"
                  value={form.acme_email} onChange={e => set('acme_email', e.target.value)} required/>
                <p className="form-hint">Let's Encrypt sends renewal notices to this address</p>
              </div>
              <div className="form-group">
                <label className="form-label">DNS provider <span style={{color:'var(--text3)'}}>optional — for DNS-01 / wildcards</span></label>
                <select className="form-select" value={form.dns_provider} onChange={e => set('dns_provider', e.target.value)}>
                  <option value="">HTTP-01 (default)</option>
                  <option value="cloudflare">Cloudflare</option>
                  <option value="route53">AWS Route 53</option>
                  <option value="digitalocean">DigitalOcean</option>
                </select>
              </div>
              {form.dns_provider && (
                <div className="form-group">
                  <label className="form-label">DNS config JSON</label>
                  <input className="form-input" placeholder='{"api_token":"..."}' value={form.dns_config}
                    onChange={e => set('dns_config', e.target.value)}/>
                </div>
              )}
              <div style={{marginTop:8}}>
                <label className="form-check">
                  <input type="checkbox" checked={form.staging || false}
                    onChange={e => set('staging', e.target.checked)}/>
                  Use staging environment (for testing — not trusted by browsers)
                </label>
                <p className="form-hint" style={{marginTop:4}}>
                  Use this to avoid Let's Encrypt rate limits during testing. Switch to production when ready.
                </p>
              </div>
            </>
          )}

          {provider === 'selfsigned' && (
            <>
              <div className="alert alert-info" style={{marginBottom:16}}>
                Self-signed certificates are not trusted by browsers. Useful for internal services or testing.
              </div>
              <div className="form-group">
                <label className="form-label">Validity period (days)</label>
                <input className="form-input" type="number" min="1" max="3650"
                  value={form.valid_days} onChange={e => set('valid_days', parseInt(e.target.value))}
                  style={{maxWidth:160}}/>
              </div>
            </>
          )}

          {provider === 'custom' && (
            <>
              <div className="form-group">
                <label className="form-label">Certificate PEM</label>
                <textarea className="form-input" rows={6} placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
                  value={form.cert_pem} onChange={e => set('cert_pem', e.target.value)} required
                  style={{fontFamily:'monospace', fontSize:11, resize:'vertical'}}/>
              </div>
              <div className="form-group">
                <label className="form-label">Private key PEM</label>
                <textarea className="form-input" rows={6} placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----"
                  value={form.key_pem} onChange={e => set('key_pem', e.target.value)} required
                  style={{fontFamily:'monospace', fontSize:11, resize:'vertical'}}/>
              </div>
            </>
          )}

          <div className="modal-footer" style={{padding:'16px 0 0', borderTop:'none'}}>
            <button type="button" className="btn" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? 'Saving…' : provider === 'letsencrypt' ? 'Request certificate' : 'Save certificate'}
            </button>
          </div>
        </form>
      )}
    </Modal>
  )
}

function RulesModal({ host, onClose }) {
  const [rules, setRules] = useState([])
  const [type, setType] = useState('ip_allow')
  const [value, setValue] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    api.listRules(host.ID).then(setRules)
  }, [host.ID])

  async function addRule(e) {
    e.preventDefault()
    setError('')
    try {
      let val = value
      if (type === 'basicauth') {
        if (!username || !password) { setError('Username and password required'); return }
        val = `${username}:${password}`
      }
      const r = await api.createRule(host.ID, { type, value: val })
      setRules(rs => [...rs, r])
      setValue(''); setUsername(''); setPassword('')
    } catch(err) { setError(err.message) }
  }

  async function deleteRule(id) {
    await api.deleteRule(host.ID, id)
    setRules(rs => rs.filter(r => r.ID !== id))
  }

  const typeLabel = { ip_allow: 'IP allow', ip_deny: 'IP deny', basicauth: 'Basic auth' }

  return (
    <Modal title={`Access rules — ${host.Domain}`} onClose={onClose}>
      {error && <div className="alert alert-error">{error}</div>}
      {rules.length > 0 ? (
        <div style={{marginBottom:16}}>
          {rules.map(r => (
            <div key={r.ID} style={{display:'flex', alignItems:'center', justifyContent:'space-between', padding:'8px 0', borderBottom:'1px solid var(--border)'}}>
              <div>
                <span className={`badge ${r.Type === 'ip_allow' ? 'badge-green' : r.Type === 'ip_deny' ? 'badge-red' : 'badge-blue'}`} style={{marginRight:8}}>
                  {typeLabel[r.Type]}
                </span>
                <code style={{fontSize:12}}>{r.Type === 'basicauth' ? r.Value.split(':')[0] : r.Value}</code>
              </div>
              <button className="btn btn-ghost btn-icon btn-sm" onClick={() => deleteRule(r.ID)}>
                <Trash2 size={13}/>
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-muted text-sm" style={{marginBottom:16}}>No access rules yet. All traffic is allowed.</p>
      )}
      <form onSubmit={addRule}>
        <div className="form-group">
          <label className="form-label">Rule type</label>
          <select className="form-select" value={type} onChange={e => { setType(e.target.value); setValue('') }}>
            <option value="ip_allow">IP allow</option>
            <option value="ip_deny">IP deny</option>
            <option value="basicauth">Basic auth</option>
          </select>
        </div>
        {type !== 'basicauth' ? (
          <div className="form-group">
            <label className="form-label">IP / CIDR</label>
            <input className="form-input" placeholder="192.168.1.0/24"
              value={value} onChange={e => setValue(e.target.value)} required/>
          </div>
        ) : (
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Username</label>
              <input className="form-input" value={username} onChange={e => setUsername(e.target.value)} required/>
            </div>
            <div className="form-group">
              <label className="form-label">Password</label>
              <input className="form-input" type="password" value={password} onChange={e => setPassword(e.target.value)} required/>
            </div>
          </div>
        )}
        <button type="submit" className="btn btn-primary btn-sm">Add rule</button>
      </form>
    </Modal>
  )
}

export function HostsPage() {
  const [hosts, setHosts] = useState([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [editHost, setEditHost] = useState(null)
  const [certHost, setCertHost] = useState(null)
  const [rulesHost, setRulesHost] = useState(null)

  async function load() {
    const h = await api.listHosts()
    setHosts(h)
  }

  useEffect(() => { load().finally(() => setLoading(false)) }, [])

  async function saveHost(form) {
    if (editHost) {
      await api.updateHost(editHost.ID, form)
    } else {
      await api.createHost(form)
    }
    setShowForm(false); setEditHost(null)
    load()
  }

  async function deleteHost(id) {
    if (!confirm('Delete this proxy host?')) return
    await api.deleteHost(id)
    load()
  }

  async function toggle(id) {
    await api.toggleHost(id)
    load()
  }

  async function toggleMaintenance(id) {
    await api.toggleMaintenance(id)
    load()
  }

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      {(showForm || editHost) && (
        <Modal title={editHost ? `Edit — ${editHost.Domain}` : 'Add proxy host'}
          onClose={() => { setShowForm(false); setEditHost(null) }}>
          <HostForm initial={editHost} onSave={saveHost} onCancel={() => { setShowForm(false); setEditHost(null) }}/>
        </Modal>
      )}
      {certHost && <CertModal host={certHost} onClose={() => { setCertHost(null); load() }} onDeleted={() => { setCertHost(null); load() }}/>}
      {rulesHost && <RulesModal host={rulesHost} onClose={() => setRulesHost(null)}/>}

      <div className="page-header-row">
        <h1>Proxy hosts</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={14}/> Add host
        </button>
      </div>

      <div className="card">
        {hosts.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon">🌐</div>
            <h3>No proxy hosts</h3>
            <p>Add a proxy host to forward traffic from a domain to an upstream service.</p>
            <button className="btn btn-primary" onClick={() => setShowForm(true)}>
              <Plus size={14}/> Add first host
            </button>
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
                  <th>Enabled</th>
                  <th>Maintenance</th>
                  <th/>
                </tr>
              </thead>
              <tbody>
                {hosts.map(h => (
                  <tr key={h.ID} style={h.MaintenanceMode ? {background:'rgba(210,153,34,0.05)'} : {}}>
                    <td>
                      <div style={{display:'flex', flexDirection:'column', gap:4}}>
                        <HealthDot status={h.HealthStatus} label="frontend" latency={h.HealthLatencyMs}/>
                        <HealthDot status={h.UpstreamStatus} label="upstream" latency={h.UpstreamLatencyMs}/>
                      </div>
                    </td>
                    <td>
                      <div>
                        <code>{h.Domain}</code>
                        {h.MaintenanceMode && <span className="badge badge-yellow" style={{marginLeft:6}}>maintenance</span>}
                      </div>
                    </td>
                    <td><span className="text-muted text-sm">{h.Upstream}</span></td>
                    <td><CertBadge certStatus={h.CertStatus} certExpiry={h.CertExpiry}/></td>
                    <td><Toggle checked={h.Enabled} onChange={() => toggle(h.ID)}/></td>
                    <td>
                      <Toggle
                        checked={h.MaintenanceMode}
                        onChange={() => toggleMaintenance(h.ID)}
                      />
                    </td>
                    <td>
                      <div className="td-actions">
                        <button className="btn btn-ghost btn-icon btn-sm" title="SSL certificate" onClick={() => setCertHost(h)}>
                          <ShieldCheck size={14}/>
                        </button>
                        <button className="btn btn-ghost btn-icon btn-sm" title="Access rules" onClick={() => setRulesHost(h)}>
                          <Lock size={14}/>
                        </button>
                        <button className="btn btn-ghost btn-icon btn-sm" title="Edit" onClick={() => setEditHost(h)}>
                          <Pencil size={14}/>
                        </button>
                        <button className="btn btn-ghost btn-icon btn-sm" title="Delete"
                          style={{color:'var(--red)'}} onClick={() => deleteHost(h.ID)}>
                          <Trash2 size={14}/>
                        </button>
                      </div>
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
