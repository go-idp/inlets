import { useEffect, useState } from 'react'
import { api, type OverviewData, type StatusData } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatTime, formatUptime } from '../lib/format'

export function StatusPage() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')
  const [overrideSize, setOverrideSize] = useState<number | null>(null)

  useEffect(() => {
    Promise.all([api.status(), api.overview()])
      .then(([s, o]) => {
        setStatus(s)
        setOverview(o)
      })
      .catch((e: Error) => setError(e.message))
    api.listOverrides().then((o) => setOverrideSize(o.size)).catch(() => { /* not configured */ })
  }, [])

  return (
    <>
      <PageHeader title="服务信息" subtitle="inlets server 运行时状态" />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      {overrideSize && overrideSize > 0 ? (
        <div className="alert" style={{ background: 'rgba(232, 184, 74, 0.10)', border: '1px solid rgba(232, 184, 74, 0.35)', color: 'var(--warn)' }}>
          ⚠ {overrideSize} 项临时覆盖生效中，进程重启后失效。
        </div>
      ) : null}
      <div className="panel">
        <table className="data kv-table">
          <tbody>
            <tr>
              <th>版本</th>
              <td>{status?.version || '—'}</td>
            </tr>
            <tr>
              <th>域名</th>
              <td>{status?.domain || '—'}</td>
            </tr>
            <tr>
              <th>HTTP 端口</th>
              <td>{status?.httpPort ?? '—'}</td>
            </tr>
            <tr>
              <th>TCP 端口</th>
              <td>{status?.tcpPort ?? '—'}</td>
            </tr>
            <tr>
              <th>HTTPS</th>
              <td>{overview?.secure ? 'enabled' : 'disabled'}</td>
            </tr>
            <tr>
              <th>启动时间</th>
              <td>{formatTime(overview?.startedAt)}</td>
            </tr>
            <tr>
              <th>运行时长</th>
              <td>{formatUptime(overview?.uptimeSeconds)}</td>
            </tr>
            <tr>
              <th>配置文件</th>
              <td>
                <code className="inline">{status?.configPath || '—'}</code>
              </td>
            </tr>
            <tr>
              <th>热重载</th>
              <td>{status?.reloadReady ? 'ready' : 'not configured'}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </>
  )
}
