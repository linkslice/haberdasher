import { useState, useEffect } from 'react'
import { api } from '../lib/api'
import { Modal } from '../components/Modal'
import { Toggle } from '../components/Toggle'
import { Plus, Pencil, Trash2, FlaskConical } from 'lucide-react'

const TYPE_DEFAULTS = {
  statsd:   { port: 8125, configJSON: '{}' },
  graphite: { port: 2003, configJSON: '{"protocol":"tcp","flush_interval_s":10}' },
  influxdb: { port: 8086, configJSON: '{"version":2,"org":"","bucket":"haberdasher","token":"","flush_interval_s":10}' },
}

function DestForm({ initial, onSave, onCancel }) {
  const [form, setForm] = useState(initial || {
    name: '', type: 'statsd', host: '', port: 8125,
    prefix: 'haberdasher', tls: false, config_json: '{}', enabled: true
  })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  function set(k, v) { setForm(f => ({...f, [k]: v})) }

  function handleTypeChange(t) {
    const d = TYPE_DEFAULTS[t]
    setForm(f => ({...f, type: t, port: d.port, config_json: d.configJSON}))
  }

  // Parse/update config JSON field nicely
  function getConfig() {
    try { return JSON.parse(form.config_json || '{}') } catch { return {} }
  }
  function setConfig(obj) { set('config_json', JSON.stringify(obj, null, 2)) }
  function setConfigKey(k, v) { setConfig({...getConfig(), [k]: v}) }

  const cfg = getConfig()

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
      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Name</label>
          <input className="form-input" placeholder="prod graphite" value={form.name}
            onChange={e => set('name', e.target.value)} required/>
        </div>
        <div className="form-group">
          <label className="form-label">Type</label>
          <select className="form-select" value={form.type} onChange={e => handleTypeChange(e.target.value)}>
            <option value="statsd">StatsD</option>
            <option value="graphite">Graphite</option>
            <option value="influxdb">InfluxDB</option>
          </select>
        </div>
      </div>
      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Host</label>
          <input className="form-input" placeholder="metrics.internal" value={form.host}
            onChange={e => set('host', e.target.value)} required/>
        </div>
        <div className="form-group">
          <label className="form-label">Port</label>
          <input className="form-input" type="number" value={form.port}
            onChange={e => set('port', parseInt(e.target.value))} required/>
        </div>
      </div>
      <div className="form-group">
        <label className="form-label">Metric prefix</label>
        <input className="form-input" placeholder="haberdasher" value={form.prefix}
          onChange={e => set('prefix', e.target.value)} required/>
        <p className="form-hint">
          {form.type !== 'influxdb'
            ? `Metrics will appear as: ${form.prefix || 'haberdasher'}.proxy.&lt;host&gt;.requests`
            : `Measurement name: ${form.prefix || 'haberdasher'}_proxy_requests`
          }
        </p>
      </div>

      {/* Type-specific config */}
      {form.type === 'graphite' && (
        <div className="form-row">
          <div className="form-group">
            <label className="form-label">Protocol</label>
            <select className="form-select" value={cfg.protocol || 'tcp'}
              onChange={e => setConfigKey('protocol', e.target.value)}>
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
            </select>
          </div>
          <div className="form-group">
            <label className="form-label">Flush interval (s)</label>
            <input className="form-input" type="number" value={cfg.flush_interval_s || 10}
              onChange={e => setConfigKey('flush_interval_s', parseInt(e.target.value))}/>
          </div>
        </div>
      )}

      {form.type === 'influxdb' && (
        <>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Version</label>
              <select className="form-select" value={cfg.version || 2}
                onChange={e => setConfigKey('version', parseInt(e.target.value))}>
                <option value={2}>InfluxDB v2</option>
                <option value={1}>InfluxDB v1</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Flush interval (s)</label>
              <input className="form-input" type="number" value={cfg.flush_interval_s || 10}
                onChange={e => setConfigKey('flush_interval_s', parseInt(e.target.value))}/>
            </div>
          </div>
          {(cfg.version || 2) === 2 ? (
            <>
              <div className="form-row">
                <div className="form-group">
                  <label className="form-label">Org</label>
                  <input className="form-input" placeholder="myorg" value={cfg.org || ''}
                    onChange={e => setConfigKey('org', e.target.value)}/>
                </div>
                <div className="form-group">
                  <label className="form-label">Bucket</label>
                  <input className="form-input" placeholder="haberdasher" value={cfg.bucket || ''}
                    onChange={e => setConfigKey('bucket', e.target.value)}/>
                </div>
              </div>
              <div className="form-group">
                <label className="form-label">Token</label>
                <input className="form-input" type="password" placeholder="your-influxdb-token"
                  value={cfg.token || ''} onChange={e => setConfigKey('token', e.target.value)}/>
              </div>
            </>
          ) : (
            <div className="form-group">
              <label className="form-label">Database</label>
              <input className="form-input" placeholder="haberdasher" value={cfg.db || ''}
                onChange={e => setConfigKey('db', e.target.value)}/>
            </div>
          )}
        </>
      )}

      <label className="form-check" style={{marginBottom:16}}>
        <input type="checkbox" checked={form.tls} onChange={e => set('tls', e.target.checked)}/>
        Use TLS (InfluxDB HTTPS)
      </label>

      <div className="modal-footer" style={{padding:'16px 0 0', borderTop:'none'}}>
        <button type="button" className="btn" onClick={onCancel}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Saving…' : 'Save destination'}
        </button>
      </div>
    </form>
  )
}

export function MonitoringPage() {
  const [dests, setDests] = useState([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [editDest, setEditDest] = useState(null)
  const [testing, setTesting] = useState({})
  const [testResult, setTestResult] = useState({})

  async function load() {
    const d = await api.listDestinations()
    setDests(d)
  }

  useEffect(() => { load().finally(() => setLoading(false)) }, [])

  async function saveDest(form) {
    if (editDest) {
      await api.updateDestination(editDest.ID, form)
    } else {
      await api.createDestination(form)
    }
    setShowForm(false); setEditDest(null)
    load()
  }

  async function deleteDest(id) {
    if (!confirm('Delete this destination?')) return
    await api.deleteDestination(id)
    load()
  }

  async function toggleDest(id) {
    await api.toggleDestination(id)
    load()
  }

  async function testDest(id) {
    setTesting(t => ({...t, [id]: true}))
    setTestResult(r => ({...r, [id]: null}))
    try {
      await api.testDestination(id)
      setTestResult(r => ({...r, [id]: 'ok'}))
    } catch(err) {
      setTestResult(r => ({...r, [id]: err.message}))
    } finally {
      setTesting(t => ({...t, [id]: false}))
      setTimeout(() => setTestResult(r => ({...r, [id]: null})), 5000)
    }
  }

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      {(showForm || editDest) && (
        <Modal title={editDest ? `Edit — ${editDest.Name}` : 'Add metric destination'}
          onClose={() => { setShowForm(false); setEditDest(null) }}>
          <DestForm initial={editDest} onSave={saveDest} onCancel={() => { setShowForm(false); setEditDest(null) }}/>
        </Modal>
      )}

      <div className="page-header-row">
        <div>
          <h1>Monitoring</h1>
        </div>
        <button className="btn btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={14}/> Add destination
        </button>
      </div>

      <div className="card" style={{marginBottom:16}}>
        <div style={{fontSize:13, color:'var(--text2)', lineHeight:1.7}}>
          <p>Per-request metrics are emitted for every proxied connection. Metric names follow the pattern:</p>
          <p style={{margin:'8px 0'}}><code>&lt;prefix&gt;.proxy.&lt;host_slug&gt;.requests</code> — plus <code>bytes_in</code>, <code>bytes_out</code>, <code>response_ms</code>, <code>status.2xx</code> etc.</p>
          <p>Set a <strong style={{color:'var(--text)'}}>metrics alias</strong> on any proxy host to control its slug in metric names.</p>
        </div>
      </div>

      <div className="card">
        {dests.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon">📊</div>
            <h3>No metric destinations</h3>
            <p>Add a StatsD, Graphite, or InfluxDB destination to start pushing metrics.</p>
            <button className="btn btn-primary" onClick={() => setShowForm(true)}>
              <Plus size={14}/> Add destination
            </button>
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Endpoint</th>
                  <th>Prefix</th>
                  <th>Enabled</th>
                  <th/>
                </tr>
              </thead>
              <tbody>
                {dests.map(d => (
                  <tr key={d.ID}>
                    <td>{d.Name}</td>
                    <td><span className="badge badge-blue">{d.Type}</span></td>
                    <td><code style={{fontSize:12}}>{d.Host}:{d.Port}{d.TLS ? ' (TLS)' : ''}</code></td>
                    <td><code style={{fontSize:12}}>{d.Prefix}</code></td>
                    <td><Toggle checked={d.Enabled} onChange={() => toggleDest(d.ID)}/></td>
                    <td>
                      <div className="td-actions">
                        {testResult[d.ID] !== undefined && testResult[d.ID] !== null && (
                          <span className={`badge ${testResult[d.ID] === 'ok' ? 'badge-green' : 'badge-red'}`} style={{marginRight:4}}>
                            {testResult[d.ID] === 'ok' ? 'sent ✓' : 'failed'}
                          </span>
                        )}
                        <button className="btn btn-ghost btn-icon btn-sm" title="Send test metric"
                          onClick={() => testDest(d.ID)} disabled={testing[d.ID]}>
                          {testing[d.ID] ? <div className="spinner" style={{width:12,height:12}}/> : <FlaskConical size={14}/>}
                        </button>
                        <button className="btn btn-ghost btn-icon btn-sm" title="Edit" onClick={() => setEditDest(d)}>
                          <Pencil size={14}/>
                        </button>
                        <button className="btn btn-ghost btn-icon btn-sm" title="Delete"
                          style={{color:'var(--red)'}} onClick={() => deleteDest(d.ID)}>
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

      <div className="card" style={{marginTop:16}}>
        <div className="card-header"><span className="card-title">Metric reference</span></div>
        <div style={{fontSize:13, color:'var(--text2)'}}>
          <table style={{width:'100%'}}>
            <thead>
              <tr>
                <th style={{textAlign:'left', padding:'4px 8px 8px 0', color:'var(--text3)', fontSize:11}}>Metric</th>
                <th style={{textAlign:'left', padding:'4px 8px 8px', color:'var(--text3)', fontSize:11}}>Type</th>
                <th style={{textAlign:'left', padding:'4px 0 8px', color:'var(--text3)', fontSize:11}}>Description</th>
              </tr>
            </thead>
            <tbody>
              {[
                ['.requests', 'counter', 'Total requests received'],
                ['.bytes_in', 'counter', 'Bytes received from client'],
                ['.bytes_out', 'counter', 'Bytes sent to client'],
                ['.response_ms', 'timer/gauge', 'Request duration in milliseconds'],
                ['.status.2xx', 'counter', 'Successful responses'],
                ['.status.4xx', 'counter', 'Client error responses'],
                ['.status.5xx', 'counter', 'Server error responses'],
              ].map(([m, t, d]) => (
                <tr key={m}>
                  <td style={{padding:'5px 8px 5px 0'}}><code style={{fontSize:11}}>&lt;prefix&gt;.proxy.&lt;host&gt;{m}</code></td>
                  <td style={{padding:'5px 8px'}}><span className="badge badge-gray">{t}</span></td>
                  <td style={{padding:'5px 0', color:'var(--text2)'}}>{d}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}
