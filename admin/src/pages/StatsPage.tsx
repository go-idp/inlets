import { useEffect, useState } from 'react'
import { api, type MetricSnapshot, type StatsData } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatBytes, formatNumber, formatTime } from '../lib/format'

export function StatsPage() {
  const [stats, setStats] = useState<StatsData | null>(null)
  const [history, setHistory] = useState<MetricSnapshot[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([api.stats(), api.statsHistory(48)])
      .then(([s, h]) => {
        setStats(s)
        setHistory(h)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  const global = stats?.global
  const clients = Object.entries(stats?.byClient ?? {})

  return (
    <>
      <PageHeader title="流量统计" subtitle="全局与按客户端聚合" />
      {error ? <div className="alert alert-danger">{error}</div> : null}

      <section className="cards">
        <div className="card">
          <div className="label">上行</div>
          <div className="value">{formatBytes(global?.uploadBytes)}</div>
        </div>
        <div className="card">
          <div className="label">下行</div>
          <div className="value">{formatBytes(global?.downloadBytes)}</div>
        </div>
        <div className="card">
          <div className="label">请求数</div>
          <div className="value">{formatNumber(global?.requests)}</div>
        </div>
        <div className="card">
          <div className="label">连接数</div>
          <div className="value">{formatNumber(global?.connections)}</div>
        </div>
      </section>

      <div className="row-2">
        <div className="panel">
          <div className="panel-head">
            <h2>按客户端</h2>
          </div>
          <table className="data">
            <thead>
              <tr>
                <th>客户端</th>
                <th>上行</th>
                <th>下行</th>
                <th>请求</th>
              </tr>
            </thead>
            <tbody>
              {clients.length === 0 ? (
                <tr>
                  <td colSpan={4} className="empty-cell">
                    暂无数据
                  </td>
                </tr>
              ) : (
                clients.map(([id, st]) => (
                  <tr key={id}>
                    <td>{id || '—'}</td>
                    <td>{formatBytes(st.uploadBytes)}</td>
                    <td>{formatBytes(st.downloadBytes)}</td>
                    <td>{formatNumber(st.requests)}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
        <div className="panel">
          <div className="panel-head">
            <h2>历史快照</h2>
          </div>
          <table className="data">
            <thead>
              <tr>
                <th>时间</th>
                <th>会话</th>
                <th>上下行</th>
              </tr>
            </thead>
            <tbody>
              {history.length === 0 ? (
                <tr>
                  <td colSpan={3} className="empty-cell">
                    暂无快照（需启用 admin.snapshotInterval）
                  </td>
                </tr>
              ) : (
                history.map((h) => (
                  <tr key={h.id}>
                    <td>{formatTime(h.createdAt)}</td>
                    <td>{h.sessionCount}</td>
                    <td>
                      ↑{formatBytes(h.uploadBytes)} ↓{formatBytes(h.downloadBytes)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}
