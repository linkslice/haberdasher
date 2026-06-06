import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useNavigate } from 'react-router-dom'
import { api, isAuthenticated } from './lib/api'
import { SetupPage } from './pages/Setup'
import { LoginPage } from './pages/Login'
import { AppShell } from './pages/AppShell'
import { DashboardPage } from './pages/Dashboard'
import { HostsPage } from './pages/Hosts'
import { CertificatesPage } from './pages/Certificates'
import { MonitoringPage } from './pages/Monitoring'
import { SettingsPage } from './pages/Settings'
import { AuditLogPage } from './pages/AuditLog'

function RequireAuth({ children }) {
  if (!isAuthenticated()) return <Navigate to="/login" replace/>
  return children
}

export default function App() {
  const [setupDone, setSetupDone] = useState(null)

  useEffect(() => {
    api.setupStatus().then(s => setSetupDone(s.complete)).catch(() => setSetupDone(true))
  }, [])

  if (setupDone === null) {
    return (
      <div style={{display:'flex', alignItems:'center', justifyContent:'center', height:'100vh'}}>
        <div className="spinner"/>
      </div>
    )
  }

  if (!setupDone) {
    return (
      <BrowserRouter>
        <Routes>
          <Route path="*" element={
            <SetupPage onComplete={() => setSetupDone(true)}/>
          }/>
        </Routes>
      </BrowserRouter>
    )
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage/>}/>
        <Route path="/setup" element={<Navigate to="/" replace/>}/>
        <Route path="/" element={<RequireAuth><AppShell/></RequireAuth>}>
          <Route index element={<DashboardPage/>}/>
          <Route path="hosts" element={<HostsPage/>}/>
          <Route path="certificates" element={<CertificatesPage/>}/>
          <Route path="monitoring" element={<MonitoringPage/>}/>
          <Route path="settings" element={<SettingsPage/>}/>
          <Route path="audit" element={<AuditLogPage/>}/>
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
