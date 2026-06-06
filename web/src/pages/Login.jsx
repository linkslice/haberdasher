import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { HaberdasherLogo } from '../components/Logo'
import { api, setToken } from '../lib/api'

export function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [needTOTP, setNeedTOTP] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await api.login(email, password, needTOTP ? totpCode : undefined)
      if (res.totp_required) {
        setNeedTOTP(true)
        setLoading(false)
        return
      }
      setToken(res.token)
      navigate('/')
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-logo">
          <HaberdasherLogo size={40}/>
          <span className="auth-logo-text">haberdasher</span>
        </div>
        <h2 className="auth-title">Sign in</h2>
        <p className="auth-sub">Reverse proxy manager</p>
        {error && <div className="alert alert-error">{error}</div>}
        <form onSubmit={handleSubmit}>
          {!needTOTP ? (
            <>
              <div className="form-group">
                <label className="form-label">Email</label>
                <input type="email" className="form-input" value={email}
                  onChange={e => setEmail(e.target.value)} required autoFocus placeholder="admin@example.com"/>
              </div>
              <div className="form-group">
                <label className="form-label">Password</label>
                <input type="password" className="form-input" value={password}
                  onChange={e => setPassword(e.target.value)} required placeholder="••••••••"/>
              </div>
            </>
          ) : (
            <div className="form-group">
              <label className="form-label">Authenticator code</label>
              <input type="text" className="form-input" value={totpCode}
                onChange={e => setTotpCode(e.target.value)} required autoFocus
                placeholder="000000" maxLength={6} inputMode="numeric"/>
              <p className="form-hint">Enter the 6-digit code from your authenticator app.</p>
            </div>
          )}
          <button type="submit" className="btn btn-primary w-full" disabled={loading}>
            {loading ? 'Signing in…' : needTOTP ? 'Verify' : 'Sign in'}
          </button>
          {needTOTP && (
            <button type="button" className="btn btn-ghost w-full mt-1"
              onClick={() => { setNeedTOTP(false); setTotpCode('') }}>
              ← Back
            </button>
          )}
        </form>
      </div>
    </div>
  )
}
