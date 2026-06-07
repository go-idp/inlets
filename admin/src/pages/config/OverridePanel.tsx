import { useEffect, useState } from 'react'
import { api } from '../../api/client'

type Props = {
  onCountChange: (n: number) => void
}

export function OverridePanel({ onCountChange }: Props) {
  const [entries, setEntries] = useState<{ path: string; value: unknown }[]>([])
  const [newPath, setNewPath] = useState('')
  const [newValue, setNewValue] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = async () => {
    try {
      const o = await api.listOverrides()
      setEntries(o.entries)
      onCountChange(o.size)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'load failed')
    }
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const onAdd = async () => {
    if (!newPath) return
    setBusy(true)
    setError('')
    try {
      let v: any = newValue
      try { v = JSON.parse(newValue) } catch { /* keep as string */ }
      await api.setOverride(newPath, v)
      setNewPath('')
      setNewValue('')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'set failed')
    } finally {
      setBusy(false)
    }
  }

  const onRemove = async (path: string) => {
    setBusy(true)
    setError('')
    try {
      await api.deleteOverride(path)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'delete failed')
    } finally {
      setBusy(false)
    }
  }

  const onClearAll = async () => {
    setBusy(true)
    setError('')
    try {
      await api.clearAllOverrides()
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'clear failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="config-section">
      <h3>运行时临时覆盖</h3>
      <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 12 }}>
        通过路径语法（<code>clients[0].clientSecret</code>）临时调整某项配置，不写文件、不持久化。
        进程重启后失效。带 &quot;临时&quot; 标记的字段会在生效期间覆盖 YAML 中的值。
      </p>
      {error ? <div className="alert alert-danger">{error}</div> : null}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <input
          className="config-field"
          style={{ flex: 2 }}
          type="text"
          placeholder="路径，例如 clients[0].clientSecret"
          value={newPath}
          onChange={(e) => setNewPath(e.target.value)}
        />
        <input
          className="config-field"
          style={{ flex: 3 }}
          type="text"
          placeholder='值，例如 "new-value" 或 8080'
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
        />
        <button type="button" className="btn btn-primary" onClick={onAdd} disabled={busy || !newPath}>
          添加覆盖
        </button>
        <button type="button" className="btn btn-secondary" onClick={onClearAll} disabled={busy || entries.length === 0}>
          清空全部
        </button>
      </div>
      {entries.length === 0 ? (
        <div className="empty-inline">当前没有覆盖项</div>
      ) : (
        <table className="data">
          <thead>
            <tr>
              <th>路径</th>
              <th>值</th>
              <th style={{ width: 60 }}></th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.path}>
                <td><code className="inline">{e.path}</code></td>
                <td><code className="inline">{JSON.stringify(e.value)}</code></td>
                <td>
                  <button type="button" className="btn-link" onClick={() => onRemove(e.path)} disabled={busy}>
                    移除
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
