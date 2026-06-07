import { useEffect, useState } from 'react'
import { api, type AuditRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { formatTime } from '../lib/format'
import { DiffViewer } from '../components/DiffViewer'

const DIFF_ACTIONS = new Set(['config.save', 'config.reload', 'config.restore'])

function isDiffy(action: string): boolean {
  return DIFF_ACTIONS.has(action)
}

export function AuditPage() {
  const [rows, setRows] = useState<AuditRow[]>([])
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  useEffect(() => {
    api
      .audit(100)
      .then(setRows)
      .catch((e: Error) => setError(e.message))
  }, [])

  const toggle = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <>
      <PageHeader title="操作审计" subtitle="来自 Admin API 的变更记录" />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div className="panel">
        <table className="data">
          <thead>
            <tr>
              <th style={{ width: 30 }}></th>
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
                <td colSpan={6} className="empty-cell">
                  暂无审计记录
                </td>
              </tr>
            ) : (
              rows.map((r) => {
                const open = expanded.has(r.id)
                const hasDiff = isDiffy(r.action) && !!r.diff && r.action === 'config.save'
                return (
                  <>
                    <tr key={r.id}>
                      <td>
                        {hasDiff ? (
                          <button
                            type="button"
                            className="btn-link"
                            onClick={() => toggle(r.id)}
                          >
                            {open ? '收起' : '展开'}
                          </button>
                        ) : null}
                      </td>
                      <td>{formatTime(r.createdAt)}</td>
                      <td>
                        <code className="inline">{r.action}</code>
                      </td>
                      <td>{r.summary}</td>
                      <td>{r.actor}</td>
                      <td>{r.clientIp}</td>
                    </tr>
                    {open && hasDiff ? (
                      <tr key={`${r.id}-diff`}>
                        <td colSpan={6}>
                          <DiffViewer raw={r.diff || ''} />
                        </td>
                      </tr>
                    ) : null}
                  </>
                )
              })
            )}
          </tbody>
        </table>
      </div>
    </>
  )
}
