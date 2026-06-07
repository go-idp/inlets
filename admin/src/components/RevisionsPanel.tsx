import { useEffect, useState } from 'react'
import { api, type RevisionRow, type RevisionDetail } from '../api/client'
import { DiffViewer, computeDiff } from './DiffViewer'

interface Props {
  currentYaml: string
  onRestored: () => void
}

export function RevisionsPanel({ currentYaml, onRestored }: Props) {
  const [revisions, setRevisions] = useState<RevisionRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<RevisionDetail | null>(null)
  const [confirmRestore, setConfirmRestore] = useState<RevisionDetail | null>(null)
  const [restoring, setRestoring] = useState(false)
  const [restoreError, setRestoreError] = useState('')

  useEffect(() => {
    let cancelled = false
    api
      .listRevisions(20)
      .then((rows) => {
        if (cancelled) return
        setRevisions(rows)
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(e instanceof Error ? e.message : 'load failed')
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [])

  const onPick = async (id: number) => {
    setError('')
    try {
      const detail = await api.getRevision(id)
      setSelected(detail)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'load failed')
    }
  }

  const onRestoreClick = (detail: RevisionDetail) => {
    setRestoreError('')
    setConfirmRestore(detail)
  }

  const onConfirmRestore = async () => {
    if (!confirmRestore) return
    setRestoring(true)
    setRestoreError('')
    try {
      await api.restoreRevision(confirmRestore.id)
      setConfirmRestore(null)
      setSelected(null)
      onRestored()
    } catch (e) {
      setRestoreError(e instanceof Error ? e.message : 'restore failed')
    } finally {
      setRestoring(false)
    }
  }

  return (
    <div className="panel">
      <div className="panel-head">
        <h2>历史版本</h2>
        <span className="link">{revisions.length} 条</span>
      </div>
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, padding: 16 }}>
        <div>
          {loading ? (
            <div className="empty-inline">加载中…</div>
          ) : revisions.length === 0 ? (
            <div className="empty-inline">暂无历史版本</div>
          ) : (
            <div className="revision-timeline">
              {revisions.map((r) => (
                <div
                  key={r.id}
                  className={`revision-item ${selected?.id === r.id ? 'active' : ''}`}
                  onClick={() => onPick(r.id)}
                >
                  <div className="revision-head">
                    <span className="revision-id">#{r.id}</span>
                    <span className="revision-when">{r.createdAt}</span>
                  </div>
                  {r.summary ? <div className="revision-summary">{r.summary}</div> : null}
                  <div className="revision-meta">
                    {r.actor} · {r.bytesSize} B
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <div>
          {selected ? (
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                <strong>版本 #{selected.id}</strong>
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={() => onRestoreClick(selected)}
                >
                  恢复此版本
                </button>
              </div>
              {selected.summary ? (
                <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 6 }}>{selected.summary}</div>
              ) : null}
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 }}>
                {selected.createdAt} · {selected.actor}
              </div>
              <h4 style={{ fontSize: 12, marginBottom: 4, color: 'var(--text-muted)' }}>当前 → 该版本</h4>
              <DiffViewer raw={computeDiff(currentYaml, selected.yaml)} />
            </div>
          ) : (
            <div className="empty-inline">选择左侧一条历史记录查看差异</div>
          )}
        </div>
      </div>

      {confirmRestore ? (
        <div className="modal-backdrop" onClick={() => !restoring && setConfirmRestore(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-head">恢复版本 #{confirmRestore.id}</div>
            <div className="modal-body">
              <p style={{ fontSize: 13, marginBottom: 12 }}>
                将使用历史版本覆盖当前配置，并触发热重载。操作会写入一条新的历史记录。
              </p>
              {restoreError ? <div className="alert alert-danger">{restoreError}</div> : null}
              <h4 style={{ fontSize: 12, marginBottom: 4, color: 'var(--text-muted)' }}>
                变更预览（当前 → 该版本）
              </h4>
              <DiffViewer raw={computeDiff(currentYaml, confirmRestore.yaml)} />
            </div>
            <div className="modal-foot">
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => setConfirmRestore(null)}
                disabled={restoring}
              >
                取消
              </button>
              <button
                type="button"
                className="btn btn-danger"
                onClick={onConfirmRestore}
                disabled={restoring}
              >
                {restoring ? '恢复中…' : '确认恢复'}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
