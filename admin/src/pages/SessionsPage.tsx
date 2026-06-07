import { useEffect, useMemo, useState } from 'react'
import { api, type SessionRow } from '../api/client'
import { PageHeader } from '../components/PageHeader'
import { shortId } from '../lib/format'

type Filter = 'all' | 'exact' | 'partial' | 'missing'

function matchLabel(m: SessionRow['configMatch']): string {
  if (m === 'exact') return '✓'
  if (m === 'partial') return '⚠'
  if (m === 'missing') return '✗'
  return '—'
}

function matchTitle(row: SessionRow): string {
  const issues = row.matchIssues ?? []
  const base =
    row.configMatch === 'exact' ? '配置已匹配'
    : row.configMatch === 'partial' ? '配置部分匹配'
    : row.configMatch === 'missing' ? '配置缺失'
    : ''
  return issues.length > 0 ? `${base}\n${issues.join('\n')}` : base
}

export function SessionsPage() {
  const [rows, setRows] = useState<SessionRow[]>([])
  const [error, setError] = useState('')
  const [filter, setFilter] = useState<Filter>('all')

  useEffect(() => {
    api
      .sessions()
      .then(setRows)
      .catch((e: Error) => setError(e.message))
  }, [])

  const filtered = useMemo(() => {
    if (filter === 'all') return rows
    return rows.filter((r) => r.configMatch === filter)
  }, [rows, filter])

  return (
    <>
      <PageHeader title="在线会话" subtitle={`共 ${rows.length} 个活跃隧道`} />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div className="panel">
        <div className="panel-head">
          <h2>会话列表</h2>
          <div style={{ display: 'flex', gap: 4 }}>
            {(['all', 'exact', 'partial', 'missing'] as Filter[]).map((f) => (
              <button
                key={f}
                type="button"
                className={filter === f ? 'btn btn-primary' : 'btn btn-secondary'}
                onClick={() => setFilter(f)}
                style={{ padding: '4px 12px', fontSize: 12 }}
              >
                {f === 'all' ? '全部' : f === 'exact' ? '匹配' : f === 'partial' ? '部分' : '缺失'}
              </button>
            ))}
          </div>
        </div>
        <table className="data">
          <thead>
            <tr>
              <th>客户端</th>
              <th>类型</th>
              <th>鉴权</th>
              <th>协议</th>
              <th>公网入口</th>
              <th>Container</th>
              <th>配置</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={7} className="empty-cell">
                  暂无符合条件的会话
                </td>
              </tr>
            ) : (
              filtered.map((s) => (
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
                  <td title={matchTitle(s)}>
                    <span style={{ marginRight: 6 }}>{matchLabel(s.configMatch)}</span>
                    {s.configIndex != null ? (
                      <code className="inline">#{s.configIndex}</code>
                    ) : (
                      <span style={{ color: 'var(--text-muted)' }}>—</span>
                    )}
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
