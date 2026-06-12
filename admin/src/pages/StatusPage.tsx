import { useEffect, useState } from 'react'
import { api, type OverviewData, type StatusData } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatTime, formatUptime } from '../lib/format'

function boolLabel(v: boolean | undefined): string {
  if (v === true) return '是'
  if (v === false) return '否'
  return '—'
}

export function StatusPage() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([api.status(), api.overview()])
      .then(([s, o]) => {
        setStatus(s)
        setOverview(o)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  const admin = status?.admin

  return (
    <>
      <PageHeader title="服务信息" subtitle="inlets server 运行时状态" />
      {error ? <div className="alert alert-danger">{error}</div> : null}
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

      <div className="panel">
        <div className="panel-head">
          <h2>Admin 控制台</h2>
        </div>
        <p style={{ margin: 0, padding: '12px 16px', fontSize: 12, color: 'var(--text-muted)', borderBottom: '1px solid var(--border)' }}>
          只读展示；修改请编辑 server.yaml 中的 <code className="inline">admin</code> 段
        </p>
        <table className="data kv-table">
          <tbody>
            <tr>
              <th>启用</th>
              <td>{boolLabel(admin?.enabled)}</td>
            </tr>
            <tr>
              <th>监听地址</th>
              <td><code className="inline">{admin?.listen || '—'}</code></td>
            </tr>
            <tr>
              <th>UI 路径</th>
              <td><code className="inline">{admin?.uiBasePath || '—'}</code></td>
            </tr>
            <tr>
              <th>数据库</th>
              <td><code className="inline">{admin?.databasePath || '—'}</code></td>
            </tr>
            <tr>
              <th>指标快照间隔</th>
              <td>{admin?.snapshotInterval || '—'}</td>
            </tr>
            <tr>
              <th>PID 文件</th>
              <td><code className="inline">{admin?.pidFile || '—'}</code></td>
            </tr>
          </tbody>
        </table>
      </div>
    </>
  )
}
