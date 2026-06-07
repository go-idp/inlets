import { useEffect, useState } from 'react'
import { api, type SessionRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { shortId } from '../lib/format'

export function SessionsPage() {
  const [rows, setRows] = useState<SessionRow[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .sessions()
      .then(setRows)
      .catch((e: Error) => setError(e.message))
  }, [])

  return (
    <>
      <PageHeader title="在线会话" subtitle={`共 ${rows.length} 个活跃隧道`} />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div className="panel">
        <table className="data">
          <thead>
            <tr>
              <th>客户端</th>
              <th>类型</th>
              <th>鉴权</th>
              <th>协议</th>
              <th>公网入口</th>
              <th>Container</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={6} className="empty-cell">
                  暂无在线会话
                </td>
              </tr>
            ) : (
              rows.map((s) => (
                <tr key={s.containerId}>
                  <td>{s.clientId || '—'}</td>
                  <td>
                    <span className={`badge badge-${s.type.toLowerCase()}`}>{s.type}</span>
                  </td>
                  <td>{s.authType || '—'}</td>
                  <td>{s.useNewProtocol ? 'v2' : 'legacy'}</td>
                  <td>{s.publicEntry || '—'}</td>
                  <td>
                    <code className="inline" title={s.containerId}>
                      {shortId(s.containerId)}
                    </code>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  )
}
