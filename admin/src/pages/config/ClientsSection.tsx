import { useState } from 'react'
import { Network, Pencil, Plus, Trash2 } from 'lucide-react'
import type { FieldDef, ValidationError } from '../../api/client'
import { Drawer } from '../../components/Drawer'
import { ClientDeleteModal } from './ClientDeleteModal'
import { ClientForm } from './ClientForm'
import { ClientTunnelsDrawer } from './ClientTunnelsDrawer'
import { SECRET_PLACEHOLDER } from './secrets'
import type { TunnelRecord } from './TunnelForm'

type ClientRecord = {
  clientId?: string
  clientSecret?: string
  tunnels?: TunnelRecord[]
}

type Props = {
  values: any
  onFieldChange: (f: FieldDef, v: unknown) => void
  errorByPath: Record<string, ValidationError>
}

type DrawerState = {
  mode: 'create' | 'edit'
  index: number
  draft: ClientRecord
}

function emptyClient(): ClientRecord {
  return { clientId: '', clientSecret: '' }
}

function tunnelSummary(client: ClientRecord): string {
  const tunnels = Array.isArray(client.tunnels) ? client.tunnels : []
  if (tunnels.length === 0) return '0'
  return String(tunnels.length)
}

function clientHasErrors(index: number, errorByPath: Record<string, ValidationError>): boolean {
  const prefix = `clients[${index}]`
  return Object.keys(errorByPath).some((p) => p === prefix || p.startsWith(`${prefix}.`))
}

export function ClientsSection({ values, onFieldChange, errorByPath }: Props) {
  const clients: ClientRecord[] = Array.isArray(values?.clients) ? values.clients : []
  const [drawer, setDrawer] = useState<DrawerState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ index: number; clientId: string } | null>(null)
  const [tunnelsTarget, setTunnelsTarget] = useState<{ index: number; clientId: string } | null>(null)

  const commitClients = (next: ClientRecord[]) => {
    onFieldChange({ path: 'clients', label: 'clients', kind: 'string' }, next)
  }

  const openCreate = () => {
    setDrawer({ mode: 'create', index: clients.length, draft: emptyClient() })
  }

  const openEdit = (index: number) => {
    const source = clients[index] ?? emptyClient()
    setDrawer({
      mode: 'edit',
      index,
      draft: {
        clientId: source.clientId ?? '',
        clientSecret: source.clientSecret === SECRET_PLACEHOLDER ? '' : (source.clientSecret ?? ''),
      },
    })
  }

  const closeDrawer = () => setDrawer(null)

  const saveDrawer = () => {
    if (!drawer) return
    const draft: ClientRecord = { clientId: drawer.draft.clientId, clientSecret: drawer.draft.clientSecret }
    if (drawer.mode === 'edit') {
      const existing = clients[drawer.index]
      if (!draft.clientSecret?.trim() || draft.clientSecret === SECRET_PLACEHOLDER) {
        draft.clientSecret = existing?.clientSecret
      }
      draft.tunnels = existing?.tunnels
      const next = [...clients]
      next[drawer.index] = draft
      commitClients(next)
    } else {
      commitClients([...clients, { ...draft, tunnels: [] }])
    }
    closeDrawer()
  }

  const confirmDelete = () => {
    if (!deleteTarget) return
    commitClients(clients.filter((_, i) => i !== deleteTarget.index))
    setDeleteTarget(null)
  }

  const saveTunnels = (index: number, tunnels: TunnelRecord[]) => {
    const next = [...clients]
    next[index] = { ...(next[index] ?? {}), tunnels }
    commitClients(next)
  }

  const drawerBase = drawer ? `clients[${drawer.index}]` : 'clients[0]'
  const drawerValid = Boolean(
    drawer?.draft.clientId?.trim()
    && (drawer.mode === 'edit' || Boolean(drawer.draft.clientSecret?.trim())),
  )

  return (
    <div className="config-section">
      <div className="config-section-head">
        <h3>客户端</h3>
        <button type="button" className="btn btn-primary" onClick={openCreate}>
          <Plus size={16} strokeWidth={1.75} />
          新增客户端
        </button>
      </div>

      {clients.length === 0 ? (
        <div className="empty-inline">暂无客户端，点击「新增客户端」添加</div>
      ) : (
        <table className="data">
          <thead>
            <tr>
              <th>Client ID</th>
              <th style={{ width: 80 }}>Tunnels</th>
              <th style={{ width: 160 }}>操作</th>
            </tr>
          </thead>
          <tbody>
            {clients.map((client, index) => (
              <tr key={`${client.clientId ?? 'row'}-${index}`}>
                <td>
                  <span className="client-id-cell">{client.clientId || '(未命名)'}</span>
                  {clientHasErrors(index, errorByPath) ? (
                    <span className="badge badge-err" style={{ marginLeft: 8 }}>!</span>
                  ) : null}
                </td>
                <td>{tunnelSummary(client)}</td>
                <td>
                  <div className="row-actions">
                    <button
                      type="button"
                      className="btn-text"
                      onClick={() => setTunnelsTarget({ index, clientId: client.clientId ?? '' })}
                    >
                      <Network size={14} strokeWidth={1.75} />
                      Tunnels
                    </button>
                    <button
                      type="button"
                      className="btn-icon"
                      title="编辑"
                      onClick={() => openEdit(index)}
                    >
                      <Pencil size={15} strokeWidth={1.75} />
                    </button>
                    <button
                      type="button"
                      className="btn-icon btn-icon-danger"
                      title="删除"
                      onClick={() => setDeleteTarget({ index, clientId: client.clientId ?? '' })}
                    >
                      <Trash2 size={15} strokeWidth={1.75} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Drawer
        open={drawer != null}
        title={drawer?.mode === 'create' ? '新增客户端' : '编辑客户端'}
        onClose={closeDrawer}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={closeDrawer}>
              取消
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={saveDrawer}
              disabled={!drawerValid}
            >
              {drawer?.mode === 'create' ? '添加' : '保存'}
            </button>
          </>
        }
      >
        {drawer ? (
          <ClientForm
            base={drawerBase}
            mode={drawer.mode}
            item={drawer.draft}
            onChange={(draft) => setDrawer({ ...drawer, draft })}
            errorByPath={errorByPath}
          />
        ) : null}
      </Drawer>

      {tunnelsTarget != null ? (
        <ClientTunnelsDrawer
          key={tunnelsTarget.index}
          clientIndex={tunnelsTarget.index}
          clientId={tunnelsTarget.clientId}
          tunnels={Array.isArray(clients[tunnelsTarget.index]?.tunnels) ? clients[tunnelsTarget.index].tunnels! : []}
          onClose={() => setTunnelsTarget(null)}
          onSave={(tunnels) => saveTunnels(tunnelsTarget.index, tunnels)}
          errorByPath={errorByPath}
        />
      ) : null}

      <ClientDeleteModal
        open={deleteTarget != null}
        clientId={deleteTarget?.clientId ?? ''}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </div>
  )
}
