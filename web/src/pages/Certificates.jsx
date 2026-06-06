import { useState, useEffect } from 'react'
import { api } from '../lib/api'
import { Trash2 } from 'lucide-react'

export function CertificatesPage() {
  const [certs, setCerts] = useState([])
  const [hosts, setHosts] = useState([])
  const [loading, setLoading] = useState(true)

  async function load() {
    const [c, h] = await Promise.all([api.listCerts(), api.listHosts()])
    setCerts(c)
    setHosts(h)
  }

  useEffect(() => { load().finally(() => setLoading(false)) }, [])

  const hostMap = Object.fromEntries(hosts.map(h => [h.ID, h]))

  async function deleteCert(cert) {
    if (!confirm(`Delete certificate for ${cert.SANs}? You can re-request it afterwards.`)) return
    await api.deleteCert(cert.ID)
    load()
  }

  function statusBadge(cert, host) {
    // Self-signed and staging always yellow
    if (cert.Provider === 'selfsigned') {
      return <span className="badge badge-yellow">self-signed</span>
    }
    if (cert.Staging) {
      return <span className="badge badge-yellow">staging</span>
    }
    // Use live host data for expiry if available
    const status = host?.CertStatus || cert.Status
    const expiry = host?.CertExpiry || (cert.Expiry ? cert.Expiry : null)

    if (status === 'active' && expiry) {
      const days = Math.floor((new Date(expiry * 1000) - Date.now()) / 86400000)
      if (days < 0)  return <span className="badge badge-red">expired</span>
      if (days < 14) return <span className="badge badge-red">expires {days}d</span>
      if (days < 30) return <span className="badge badge-yellow">{days}d left</span>
      return <span className="badge badge-green">valid</span>
    }
    if (status === 'pending') return <span className="badge badge-yellow">pending</span>
    if (status === 'none' || !status) return <span className="badge badge-gray">not requested</span>
    return <span className="badge badge-gray">{status}</span>
  }

  function providerBadge(cert) {
    const map = {
      letsencrypt: cert.Staging
        ? <span className="badge badge-yellow">Let's Encrypt (staging)</span>
        : <span className="badge badge-blue">Let's Encrypt</span>,
      selfsigned:  <span className="badge badge-yellow">self-signed</span>,
      custom:      <span className="badge badge-blue">custom</span>,
    }
    return map[cert.Provider] || <span className="badge badge-gray">{cert.Provider}</span>
  }

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      <div className="page-header">
        <h1>Certificates</h1>
        <p>TLS certificates managed by Haberdasher. Status is verified live via HTTPS health checks.</p>
      </div>

      <div className="card">
        {certs.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon">🔒</div>
            <h3>No certificates yet</h3>
            <p>Go to Proxy Hosts, click the shield icon on a host, and request a certificate.</p>
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Domain</th>
                  <th>Provider</th>
                  <th>ACME email</th>
                  <th>Expiry</th>
                  <th>Status</th>
                  <th/>
                </tr>
              </thead>
              <tbody>
                {certs.map(c => {
                  const host = hostMap[c.HostID]
                  const expiry = host?.CertExpiry || c.Expiry
                  return (
                    <tr key={c.ID}>
                      <td><code>{host?.Domain || c.SANs}</code></td>
                      <td>{providerBadge(c)}</td>
                      <td><span className="text-muted text-sm">{c.ACMEEmail || '—'}</span></td>
                      <td>
                        <span className="text-sm">
                          {expiry ? new Date(expiry * 1000).toLocaleDateString() : '—'}
                        </span>
                      </td>
                      <td>{statusBadge(c, host)}</td>
                      <td>
                        <button className="btn btn-ghost btn-icon btn-sm"
                          title="Delete certificate" style={{color:'var(--red)'}}
                          onClick={() => deleteCert(c)}>
                          <Trash2 size={13}/>
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{marginTop:16}}>
        <div className="card-header"><span className="card-title">Certificate types</span></div>
        <div style={{fontSize:13, lineHeight:1.7, color:'var(--text2)'}}>
          <ul style={{paddingLeft:20, display:'flex', flexDirection:'column', gap:6}}>
            <li><strong style={{color:'var(--text)'}}>Let's Encrypt</strong> — Automatic ACME via HTTP-01 or DNS-01. Free, auto-renewed by Caddy 30 days before expiry.</li>
            <li><strong style={{color:'var(--text)'}}>Self-signed</strong> — Generated instantly, no CA required. Not trusted by browsers — use for internal services or testing only. Shows yellow in the UI as a reminder.</li>
            <li><strong style={{color:'var(--text)'}}>Custom</strong> — Paste your own cert and key PEM. Useful for certs purchased from a commercial CA.</li>
          </ul>
        </div>
      </div>
    </>
  )
}
