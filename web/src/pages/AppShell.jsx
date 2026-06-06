import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { HaberdasherLogo } from '../components/Logo'
import { clearToken } from '../lib/api'
import { useTheme } from '../hooks/useTheme'
import {
  LayoutDashboard, Globe, Shield, Activity,
  Settings, ScrollText, LogOut, Sun, Moon
} from 'lucide-react'

export function AppShell() {
  const navigate = useNavigate()
  const { theme, toggle } = useTheme()

  function logout() {
    clearToken()
    navigate('/login')
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-logo">
          <HaberdasherLogo size={28}/>
          <div>
            <div className="sidebar-logo-text">haberdasher</div>
            <div className="sidebar-logo-sub">proxy manager</div>
          </div>
        </div>

        <nav className="sidebar-nav">
          <div className="nav-section">Main</div>
          <NavLink to="/" end className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <LayoutDashboard size={15}/> Dashboard
          </NavLink>
          <NavLink to="/hosts" className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <Globe size={15}/> Proxy Hosts
          </NavLink>
          <NavLink to="/certificates" className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <Shield size={15}/> Certificates
          </NavLink>

          <div className="nav-section">Observability</div>
          <NavLink to="/monitoring" className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <Activity size={15}/> Monitoring
          </NavLink>
          <NavLink to="/audit" className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <ScrollText size={15}/> Audit Log
          </NavLink>

          <div className="nav-section">Admin</div>
          <NavLink to="/settings" className={({isActive}) => 'nav-item' + (isActive ? ' active' : '')}>
            <Settings size={15}/> Settings
          </NavLink>
        </nav>

        <div className="sidebar-footer">
          <div style={{display:'flex', alignItems:'center', justifyContent:'space-between'}}>
            <button
              onClick={logout}
              style={{display:'flex', alignItems:'center', gap:6, background:'none', border:'none',
                cursor:'pointer', color:'var(--text3)', fontSize:13, padding:'4px 0'}}
            >
              <LogOut size={14}/> Sign out
            </button>
            <button
              onClick={toggle}
              title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
              style={{display:'flex', alignItems:'center', justifyContent:'center',
                background:'none', border:'1px solid var(--border)', borderRadius:6,
                cursor:'pointer', color:'var(--text2)', padding:'4px 6px',
                transition:'all 0.15s'}}
            >
              {theme === 'dark' ? <Sun size={14}/> : <Moon size={14}/>}
            </button>
          </div>
        </div>
      </aside>

      <main className="main-content">
        <Outlet/>
      </main>
    </div>
  )
}
