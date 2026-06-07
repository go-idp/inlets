import { useEffect, useState } from 'react'
import { api, type AuditRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatTime } from '../lib/format'

export function AuditPage() {
  const [rows, setRows] = useState<AuditRow[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .audit(100)
      .then(setRows)
      .catch((e: Error) => setError(e.message))
  }, [])

  return (
    <>
      <PageHeader title="操作审计" subtitle="来自 Admin API 的变更记录" />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div className="panel">
        <table className="data">
          <thead>
            <tr>
              <th>时间</th>
              <th>操作</th>
              <th>摘要</th>
              <th>操作者</th>
              <th>IP</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={5} className="empty-cell">
                  暂无审计记录
                </td>
              </tr>
            ) : (
              rows.map((r) => (
                <tr key={r.id}>
                  <td>{formatTime(r.createdAt)}</td>
                  <td>
                    <code className="inline">{r.action}</code>
                  </td>
                  <td>{r.summary}</td>
                  <td>{r.actor}</td>
                  <td>{r.clientIp}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  )
}
