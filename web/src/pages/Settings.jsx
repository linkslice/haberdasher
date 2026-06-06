import { useState, useEffect } from 'react'
import { api } from '../lib/api'

export function SettingsPage() {
  const [user, setUser] = useState(null)
  const [settings, setSettings] = useState({})
  const [instanceName, setInstanceName] = useState('')
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwMsg, setPwMsg] = useState(null)
  const [settingsMsg, setSettingsMsg] = useState(null)
  const [totpSetup, setTotpSetup] = useState(null)
  const [totpCode, setTotpCode] = useState('')
  const [totpMsg, setTotpMsg] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([api.me(), api.getSettings()])
      .then(([u, s]) => {
        setUser(u)
        setSettings(s)
        setInstanceName(s.instance_name || '')
      })
      .finally(() => setLoading(false))
  }, [])

  async function saveSettings(e) {
    e.preventDefault()
    setSettingsMsg(null)
    try {
      await api.updateSettings({ instance_name: instanceName })
      setSettingsMsg({ ok: true, msg: 'Settings saved.' })
    } catch(err) { setSettingsMsg({ ok: false, msg: err.message }) }
  }

  async function changePassword(e) {
    e.preventDefault()
    setPwMsg(null)
    if (newPw !== confirmPw) { setPwMsg({ ok: false, msg: 'Passwords do not match' }); return }
    if (newPw.length < 8) { setPwMsg({ ok: false, msg: 'Password must be at least 8 characters' }); return }
    try {
      await api.changePassword(currentPw, newPw)
      setPwMsg({ ok: true, msg: 'Password changed.' })
      setCurrentPw(''); setNewPw(''); setConfirmPw('')
    } catch(err) { setPwMsg({ ok: false, msg: err.message }) }
  }

  async function startTotpSetup() {
    setTotpMsg(null)
    const res = await api.totpSetup()
    setTotpSetup(res)
  }

  async function confirmTotp(e) {
    e.preventDefault()
    setTotpMsg(null)
    try {
      await api.totpConfirm(totpCode)
      setTotpMsg({ ok: true, msg: 'Two-factor authentication enabled.' })
      setTotpSetup(null); setTotpCode('')
      const u = await api.me()
      setUser(u)
    } catch(err) { setTotpMsg({ ok: false, msg: err.message }) }
  }

  async function disableTotp() {
    if (!confirm('Disable two-factor authentication?')) return
    await api.totpDisable()
    const u = await api.me()
    setUser(u)
    setTotpMsg({ ok: true, msg: 'Two-factor authentication disabled.' })
  }

  if (loading) return <div className="loading-center"><div className="spinner"/></div>

  return (
    <>
      <div className="page-header">
        <h1>Settings</h1>
        <p>Instance configuration and account security</p>
      </div>

      {/* Instance settings */}
      <div className="card">
        <div className="card-header"><span className="card-title">Instance</span></div>
        {settingsMsg && <div className={`alert ${settingsMsg.ok ? 'alert-success' : 'alert-error'}`}>{settingsMsg.msg}</div>}
        <form onSubmit={saveSettings}>
          <div className="form-group">
            <label className="form-label">Instance name</label>
            <input className="form-input" placeholder="prod-proxy-01" value={instanceName}
              onChange={e => setInstanceName(e.target.value)}/>
            <p className="form-hint">Used as the <code>instance</code> tag in InfluxDB metrics</p>
          </div>
          <div className="form-group">
            <label className="form-label">Admin email</label>
            <input className="form-input" value={settings.admin_email || ''} readOnly/>
          </div>
          <button type="submit" className="btn btn-primary">Save</button>
        </form>
      </div>

      {/* Password change */}
      <div className="card" style={{marginTop:16}}>
        <div className="card-header"><span className="card-title">Change password</span></div>
        {pwMsg && <div className={`alert ${pwMsg.ok ? 'alert-success' : 'alert-error'}`}>{pwMsg.msg}</div>}
        <form onSubmit={changePassword}>
          <div className="form-group">
            <label className="form-label">Current password</label>
            <input className="form-input" type="password" value={currentPw}
              onChange={e => setCurrentPw(e.target.value)} required/>
          </div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">New password</label>
              <input className="form-input" type="password" value={newPw}
                onChange={e => setNewPw(e.target.value)} required/>
            </div>
            <div className="form-group">
              <label className="form-label">Confirm</label>
              <input className="form-input" type="password" value={confirmPw}
                onChange={e => setConfirmPw(e.target.value)} required/>
            </div>
          </div>
          <button type="submit" className="btn btn-primary">Change password</button>
        </form>
      </div>

      {/* TOTP */}
      <div className="card" style={{marginTop:16}}>
        <div className="card-header">
          <span className="card-title">Two-factor authentication</span>
          <span className={`badge ${user?.totp_enabled ? 'badge-green' : 'badge-gray'}`}>
            {user?.totp_enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        {totpMsg && <div className={`alert ${totpMsg.ok ? 'alert-success' : 'alert-error'}`}>{totpMsg.msg}</div>}
        {!user?.totp_enabled ? (
          <>
            {!totpSetup ? (
              <div>
                <p className="text-muted text-sm" style={{marginBottom:12}}>
                  Add an extra layer of security with a TOTP authenticator app (Google Authenticator, Authy, etc.).
                </p>
                <button className="btn btn-primary" onClick={startTotpSetup}>Set up 2FA</button>
              </div>
            ) : (
              <div>
                <p className="text-muted text-sm" style={{marginBottom:16}}>
                  Scan this QR code with your authenticator app, then enter the 6-digit code to confirm.
                </p>
                <div style={{
                  background:'white', padding:16, borderRadius:8, display:'inline-block', marginBottom:16
                }}>
                  <img src={`https://api.qrserver.com/v1/create-qr-code/?size=180x180&data=${encodeURIComponent(totpSetup.otpauth_url)}`}
                    alt="TOTP QR code" width={180} height={180}/>
                </div>
                <div className="form-group">
                  <label className="form-label">Manual entry key</label>
                  <code style={{display:'block', padding:'8px 10px', background:'var(--bg3)', borderRadius:6, letterSpacing:2}}>
                    {totpSetup.secret}
                  </code>
                </div>
                <form onSubmit={confirmTotp}>
                  <div className="form-group">
                    <label className="form-label">Verification code</label>
                    <input className="form-input" placeholder="000000" maxLength={6} inputMode="numeric"
                      value={totpCode} onChange={e => setTotpCode(e.target.value)} required style={{maxWidth:160}}/>
                  </div>
                  <div className="flex gap-2">
                    <button type="button" className="btn" onClick={() => setTotpSetup(null)}>Cancel</button>
                    <button type="submit" className="btn btn-primary">Confirm and enable</button>
                  </div>
                </form>
              </div>
            )}
          </>
        ) : (
          <div>
            <p className="text-muted text-sm" style={{marginBottom:12}}>
              Two-factor authentication is active. You'll be prompted for a code on each login.
            </p>
            <button className="btn btn-danger" onClick={disableTotp}>Disable 2FA</button>
          </div>
        )}
      </div>
    </>
  )
}
