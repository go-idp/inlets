import { Navigate, Route, Routes } from 'react-router-dom'
import { AppLayout } from './layout/AppLayout'
import { AuditPage } from './pages/AuditPage'
import { ConfigPage } from './pages/ConfigPage'
import { DomainsPage } from './pages/DomainsPage'
import { OverviewPage } from './pages/OverviewPage'
import { SessionsPage } from './pages/SessionsPage'
import { StatsPage } from './pages/StatsPage'
import { StatusPage } from './pages/StatusPage'

export function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<OverviewPage />} />
        <Route path="sessions" element={<SessionsPage />} />
        <Route path="domains" element={<DomainsPage />} />
        <Route path="stats" element={<StatsPage />} />
        <Route path="config" element={<ConfigPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="status" element={<StatusPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
