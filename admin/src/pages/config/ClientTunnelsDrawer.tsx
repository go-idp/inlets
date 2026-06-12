import { useState } from 'react'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import type { ValidationError } from '../../api/client'
import { Drawer } from '../../components/Drawer'
import { emptyTunnel, TunnelForm, type TunnelRecord } from './TunnelForm'

type Props = {
  clientIndex: number
  clientId: string
  tunnels: TunnelRecord[]
  onClose: () => void
  onSave: (tunnels: TunnelRecord[]) => void
  errorByPath: Record<string, ValidationError>
}

type TunnelDrawerState = {
  mode: 'create' | 'edit'
  index: number
  draft: TunnelRecord
}

function tunnelLabel(t: TunnelRecord): string {
  const type = t.type || '?'
  const upstream = t.upstream || '?'
  return t.name ? `${t.name} (${type} → ${upstream})` : `${type} → ${upstream}`
}

function normalizeTunnels(tunnels: TunnelRecord[]): TunnelRecord[] {
  return tunnels.map((t) => {
    const type = (t.type ?? '').toLowerCase()
    const next: TunnelRecord = { ...t, type }
    if (type === 'http') {
      delete next.remotePort
    } else if (type === 'tcp') {
      delete next.subDomain
    }
    if (!next.name?.trim()) {
      delete next.name
    }
    return next
  })
}

export function ClientTunnelsDrawer({
  clientIndex, clientId, tunnels, onClose, onSave, errorByPath,
}: Props) {
  const base = `clients[${clientIndex}]`
  const [localTunnels, setLocalTunnels] = useState<TunnelRecord[]>(tunnels)
  const [tunnelDrawer, setTunnelDrawer] = useState<TunnelDrawerState | null>(null)
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)

  const openCreate = () => {
    setTunnelDrawer({ mode: 'create', index: localTunnels.length, draft: emptyTunnel() })
  }

  const openEdit = (index: number) => {
    setTunnelDrawer({
      mode: 'edit',
      index,
      draft: { ...(localTunnels[index] ?? emptyTunnel()) },
    })
  }

  const saveTunnelDrawer = () => {
    if (!tunnelDrawer) return
    const draft = normalizeTunnels([tunnelDrawer.draft])[0]
    const next = [...localTunnels]
    if (tunnelDrawer.mode === 'create') {
      next.push(draft)
    } else {
      next[tunnelDrawer.index] = draft
    }
    setLocalTunnels(next)
    setTunnelDrawer(null)
  }

  const confirmDelete = () => {
    if (deleteIndex == null) return
    setLocalTunnels(localTunnels.filter((_, i) => i !== deleteIndex))
    setDeleteIndex(null)
  }

  const handleClose = () => {
    onClose()
  }

  const handleSaveAll = () => {
    onSave(normalizeTunnels(localTunnels))
    onClose()
  }

  const tunnelValid = Boolean(
    tunnelDrawer?.draft.type?.trim() && tunnelDrawer?.draft.upstream?.trim(),
  )

  return (
    <>
      <Drawer
        open
        title={`Tunnels · ${clientId || '(未命名)'}`}
        onClose={handleClose}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={handleClose}>
              取消
            </button>
            <button type="button" className="btn btn-primary" onClick={handleSaveAll}>
              保存
            </button>
          </>
        }
      >
        <div className="config-section-head" style={{ marginBottom: 12 }}>
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
            {localTunnels.length} 条隧道
          </span>
          <button type="button" className="btn btn-primary" onClick={openCreate}>
            <Plus size={16} strokeWidth={1.75} />
            新增 Tunnel
          </button>
        </div>

        {localTunnels.length === 0 ? (
          <div className="empty-inline">暂无 Tunnel，点击「新增 Tunnel」添加</div>
        ) : (
          <table className="data">
            <thead>
              <tr>
                <th>名称 / 类型</th>
                <th>上游</th>
                <th>子域 / 端口</th>
                <th style={{ width: 88 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {localTunnels.map((t, index) => (
                <tr key={`${t.name ?? ''}-${t.upstream ?? ''}-${index}`}>
                  <td>
                    <div>{t.name || '—'}</div>
                    <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>{t.type || '—'}</div>
                  </td>
                  <td><code className="inline">{t.upstream || '—'}</code></td>
                  <td>
                    {t.type === 'tcp'
                      ? (t.remotePort != null && t.remotePort !== '' ? String(t.remotePort) : '—')
                      : (t.subDomain || '—')}
                  </td>
                  <td>
                    <div className="row-actions">
                      <button type="button" className="btn-icon" title="编辑" onClick={() => openEdit(index)}>
                        <Pencil size={15} strokeWidth={1.75} />
                      </button>
                      <button type="button" className="btn-icon btn-icon-danger" title="删除" onClick={() => setDeleteIndex(index)}>
                        <Trash2 size={15} strokeWidth={1.75} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Drawer>

      <Drawer
        open={tunnelDrawer != null}
        nested
        title={tunnelDrawer?.mode === 'create' ? '新增 Tunnel' : '编辑 Tunnel'}
        onClose={() => setTunnelDrawer(null)}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={() => setTunnelDrawer(null)}>
              取消
            </button>
            <button type="button" className="btn btn-primary" onClick={saveTunnelDrawer} disabled={!tunnelValid}>
              {tunnelDrawer?.mode === 'create' ? '添加' : '保存'}
            </button>
          </>
        }
      >
        {tunnelDrawer ? (
          <TunnelForm
            base={base}
            tunnelIndex={tunnelDrawer.index}
            item={tunnelDrawer.draft}
            onChange={(draft) => setTunnelDrawer({ ...tunnelDrawer, draft })}
            errorByPath={errorByPath}
          />
        ) : null}
      </Drawer>

      {deleteIndex != null ? (
        <div className="modal-backdrop" onClick={() => setDeleteIndex(null)}>
          <div className="modal modal-sm" onClick={(e) => e.stopPropagation()}>
            <div className="modal-head">删除 Tunnel</div>
            <div className="modal-body">
              <p style={{ fontSize: 13, lineHeight: 1.6 }}>
                确定删除 <code className="inline">{tunnelLabel(localTunnels[deleteIndex] ?? {})}</code> 吗？
              </p>
            </div>
            <div className="modal-foot">
              <button type="button" className="btn btn-secondary" onClick={() => setDeleteIndex(null)}>
                取消
              </button>
              <button type="button" className="btn btn-danger" onClick={confirmDelete}>
                确认删除
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  )
}
