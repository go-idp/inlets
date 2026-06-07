import { useEffect, useState } from 'react'
import { api, type DomainRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'

export function DomainsPage() {
  const [rows, setRows] = useState<DomainRow[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .domains()
      .then(setRows)
      .catch((e: Error) => setError(e.message))
  }, [])

  return (
    <>
      <PageHeader title="子域映射" subtitle={`共 ${rows.length} 个子域`} />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div className="panel">
        <table className="data">
          <thead>
            <tr>
              <th>子域</th>
              <th>客户端</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={2} className="empty-cell">
                  暂无子域映射
                </td>
              </tr>
            ) : (
              rows.map((d) => (
                <tr key={d.subDomain}>
                  <td>
                    <code className="inline">{d.subDomain}</code>
                  </td>
                  <td>{d.clientId || '—'}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  )
}
