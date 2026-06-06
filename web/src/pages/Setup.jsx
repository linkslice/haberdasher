import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { HaberdasherLogo } from '../components/Logo'
import { api, setToken } from '../lib/api'

export function SetupPage({ onComplete }) {
  const [step, setStep] = useState(1)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    if (password !== confirm) { setError('Passwords do not match'); return }
    if (password.length < 8) { setError('Password must be at least 8 characters'); return }
    setLoading(true)
    try {
      const res = await api.setupComplete(email, password)
      setToken(res.token)
      onComplete && onComplete()
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
          <HaberdasherLogo size={40} />
          <span className="auth-logo-text">haberdasher</span>
        </div>

        <div className="wizard-steps" style={{marginBottom: 24}}>
          <div className={`wizard-step ${step >= 1 ? 'active' : ''}`}/>
          <div className={`wizard-step ${step >= 2 ? 'active' : ''}`}/>
          <div className={`wizard-step ${step >= 3 ? 'active' : ''}`}/>
        </div>

        {step === 1 && (
          <>
            <h2 className="auth-title">Welcome to Haberdasher</h2>
            <p className="auth-sub">Lightweight reverse proxy manager. Let's set up your admin account.</p>
            <button className="btn btn-primary w-full" onClick={() => setStep(2)}>
              Get started
            </button>
          </>
        )}

        {step === 2 && (
          <form onSubmit={e => { e.preventDefault(); if (!email) return; setStep(3) }}>
            <h2 className="auth-title">Admin email</h2>
            <p className="auth-sub" style={{marginBottom: 20}}>This will be your login and the default ACME contact for Let's Encrypt.</p>
            <div className="form-group">
              <label className="form-label">Email address</label>
              <input
                type="email"
                className="form-input"
                placeholder="admin@example.com"
                value={email}
                onChange={e => setEmail(e.target.value)}
                required
                autoFocus
              />
            </div>
            <div className="flex gap-2">
              <button type="button" className="btn" onClick={() => setStep(1)}>Back</button>
              <button type="submit" className="btn btn-primary" style={{flex:1}}>Continue</button>
            </div>
          </form>
        )}

        {step === 3 && (
          <form onSubmit={handleSubmit}>
            <h2 className="auth-title">Set admin password</h2>
            <p className="auth-sub" style={{marginBottom: 20}}>Choose a strong password for <strong>{email}</strong>.</p>
            {error && <div className="alert alert-error">{error}</div>}
            <div className="form-group">
              <label className="form-label">Password</label>
              <input
                type="password"
                className="form-input"
                placeholder="At least 8 characters"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
                autoFocus
              />
            </div>
            <div className="form-group">
              <label className="form-label">Confirm password</label>
              <input
                type="password"
                className="form-input"
                placeholder="Repeat password"
                value={confirm}
                onChange={e => setConfirm(e.target.value)}
                required
              />
            </div>
            <div className="flex gap-2">
              <button type="button" className="btn" onClick={() => setStep(2)}>Back</button>
              <button type="submit" className="btn btn-primary" style={{flex:1}} disabled={loading}>
                {loading ? 'Setting up…' : 'Complete setup'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}
