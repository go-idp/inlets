import { useCallback, useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { api } from '../api/client'
import { navGroups } from './navConfig'

export function AppLayout() {
  const [configPath, setConfigPath] = useState('—')
  const [reloadReady, setReloadReady] = useState(false)
  const [sessionCount, setSessionCount] = useState(0)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const location = useLocation()

  const loadStatus = useCallback(() => {
    api
      .status()
      .then((s) => {
        setConfigPath(s.configPath || '—')
        setReloadReady(Boolean(s.reloadReady))
        setSessionCount(s.sessionCount ?? 0)
      })
      .catch(() => {
        setConfigPath('—')
        setReloadReady(false)
        setSessionCount(0)
      })
  }, [])

  useEffect(() => {
    setDrawerOpen(false)
  }, [location.pathname])

  useEffect(() => {
    loadStatus()
    const timer = window.setInterval(loadStatus, 30_000)
    return () => window.clearInterval(timer)
  }, [loadStatus])

  const close = () => setDrawerOpen(false)

  return (
    <>
      <div className="mobile-topbar">
        <button
          type="button"
          className="mobile-hamburger"
          aria-label="打开导航"
          aria-expanded={drawerOpen}
          onClick={() => setDrawerOpen((v) => !v)}
        >
          <span className="hamburger-line" />
          <span className="hamburger-line" />
          <span className="hamburger-line" />
        </button>
        <span className="mobile-topbar-title">Inlets Console</span>
      </div>

      {drawerOpen ? (
        <div className="sidebar-backdrop" onClick={close} role="presentation" />
      ) : null}

      <div className="layout">
        <aside className={`sidebar${drawerOpen ? ' open' : ''}`}>
          <div className="brand">
            Inlets Console
            <span>隧道运维管理</span>
          </div>
          <nav className="nav" aria-label="主导航">
            {navGroups.map((group) => (
              <div key={group.label} className="nav-group">
                <div className="nav-group-label">{group.label}</div>
                {group.items.map((item) => {
                  const Icon = item.icon
                  return (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      end={item.end}
                      className={({ isActive }) => (isActive ? 'active' : '')}
                      onClick={close}
                    >
                      <span className="icon" aria-hidden>
                        <Icon size={18} strokeWidth={1.75} />
                      </span>
                      <span className="nav-label">{item.label}</span>
                    </NavLink>
                  )
                })}
              </div>
            ))}
          </nav>
          <div className="sidebar-footer">
            <div className="status-line">
              <span className="status-dot-sm" />
              {sessionCount} 会话在线 · reload {reloadReady ? '就绪' : '未配置'}
            </div>
            配置路径
            <br />
            <code title={configPath}>{configPath}</code>
          </div>
        </aside>
        <main className="main">
          <Outlet />
        </main>
      </div>
    </>
  )
}
