import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type OverviewData, type SessionRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatBytes, formatNumber, formatUptime, shortId } from '../lib/format'

export function OverviewPage() {
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [history, setHistory] = useState<number[]>([])
  const [domains, setDomains] = useState<{ subDomain: string; clientId: string }[]>([])
  const [error, setError] = useState('')
  const [refreshAt, setRefreshAt] = useState('')
  const [reloading, setReloading] = useState(false)

  const load = useCallback(() => {
    setError('')
    Promise.all([api.overview(), api.sessions(), api.domains(), api.statsHistory(24)])
      .then(([ov, sess, doms, hist]) => {
        setOverview(ov)
        setSessions(sess.slice(0, 5))
        setDomains(doms.slice(0, 5))
        const bars = [...hist].reverse().map((h) => h.downloadBytes + h.uploadBytes)
        const max = Math.max(...bars, 1)
        setHistory(bars.map((v) => Math.round((v / max) * 100)))
        setRefreshAt(new Date().toLocaleTimeString())
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  useEffect(() => {
    load()
    const t = window.setInterval(load, 15_000)
    return () => window.clearInterval(t)
  }, [load])

  const onReload = async () => {
    setReloading(true)
    try {
      await api.reload()
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'reload failed')
    } finally {
      setReloading(false)
    }
  }

  const stats = overview?.stats
  const subtitle = overview
    ? `${overview.domain || '—'} · 运行 ${formatUptime(overview.uptimeSeconds)} · 刷新 ${refreshAt || '—'}`
    : '加载中…'

  return (
    <>
      <PageHeader
        title="运行概览"
        subtitle={subtitle}
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={load}>
              刷新
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={reloading}
              onClick={onReload}
            >
              {reloading ? '重载中…' : '重载配置'}
            </button>
          </>
        }
      />

      {error ? <div className="alert alert-danger">{error}</div> : null}

      <section className="cards">
        <div className="card">
          <div className="label">在线会话</div>
          <div className="value">{formatNumber(overview?.sessionCount)}</div>
        </div>
        <div className="card">
          <div className="label">上行流量</div>
          <div className="value">{formatBytes(stats?.uploadBytes)}</div>
        </div>
        <div className="card">
          <div className="label">下行流量</div>
          <div className="value">{formatBytes(stats?.downloadBytes)}</div>
        </div>
        <div className="card">
          <div className="label">HTTP 请求</div>
          <div className="value">{formatNumber(stats?.requests)}</div>
        </div>
        <div className="card">
          <div className="label">子域数量</div>
          <div className="value">{formatNumber(overview?.domainCount)}</div>
        </div>
      </section>

      <div className="row-2">
        <div className="panel">
          <div className="panel-head">
            <h2>在线会话列表</h2>
            <Link className="link" to="/sessions">
              查看全部 →
            </Link>
          </div>
          <table className="data">
            <thead>
              <tr>
                <th>状态</th>
                <th>客户端</th>
                <th>类型</th>
                <th>公网入口</th>
                <th>Container</th>
              </tr>
            </thead>
            <tbody>
              {sessions.length === 0 ? (
                <tr>
                  <td colSpan={5} className="empty-cell">
                    暂无在线会话
                  </td>
                </tr>
              ) : (
                sessions.map((s) => (
                  <tr key={s.containerId}>
                    <td>
                      <span className="status-dot" />
                      运行中
                    </td>
                    <td>{s.clientId || '—'}</td>
                    <td>
                      <span className={`badge badge-${s.type.toLowerCase()}`}>{s.type}</span>
                    </td>
                    <td>{s.publicEntry || '—'}</td>
                    <td>
                      <code className="inline">{shortId(s.containerId)}</code>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
        <div>
          <div className="panel">
            <div className="panel-head">
              <h2>流量快照趋势</h2>
            </div>
            <div className="timeline-chart">
              {history.length === 0 ? (
                <div className="empty-inline">暂无历史快照</div>
              ) : (
                history.map((h, i) => (
                  <div key={i} className="timeline-col">
                    <div className="timeline-bar-area">
                      <div className="timeline-stack" style={{ height: `${Math.max(h, 4)}%` }} />
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
          <div className="panel">
            <div className="panel-head">
              <h2>子域映射 Top 5</h2>
            </div>
            <div className="domain-list">
              {domains.length === 0 ? (
                <div className="empty-inline">暂无子域映射</div>
              ) : (
                domains.map((d) => (
                  <div key={d.subDomain}>
                    <code>{d.subDomain}</code>
                    <span>{d.clientId || '—'}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </>
  )
}
