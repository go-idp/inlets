import { useEffect, useMemo, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import type { ConfigSchema, FieldDef, GroupDef, ValidationError } from '../../api/client'
import { ClientsSection } from './ClientsSection'
import { ConfigSectionCard } from './ConfigSectionCard'

type Props = {
  schema: ConfigSchema
  values: any
  onFieldChange: (f: FieldDef, v: unknown) => void
  errorByPath: Record<string, ValidationError>
}

type SidebarEntry =
  | { kind: 'group'; key: string; group: GroupDef; errorCount: number }
  | { kind: 'clients'; key: string }

export function ConfigForm({ schema, values, onFieldChange, errorByPath }: Props) {
  // Top-level groups: object groups plus the "clients" pseudo-group.
  const topGroups: GroupDef[] = useMemo(
    () => schema.groups.filter(
      (g) => g.kind === 'object' && g.key !== 'clients.item' && g.key !== 'tunnels.item',
    ),
    [schema],
  )

  const errorCountForGroup = (g: GroupDef) =>
    Object.keys(errorByPath).filter((p) => p.startsWith(g.path === '' ? '' : `${g.path}.`)).length

  const errorCountForClients = () =>
    Object.keys(errorByPath).filter((p) => p.startsWith('clients')).length

  const entries: SidebarEntry[] = useMemo(() => {
    const out: SidebarEntry[] = topGroups.map((g) => ({
      kind: 'group', key: g.key, group: g, errorCount: errorCountForGroup(g),
    }))
    out.push({ kind: 'clients', key: 'clients' })
    return out
  }, [topGroups, errorByPath])

  // Default the active section to the first one.
  const [activeKey, setActiveKey] = useState<string>(entries[0]?.key ?? '')

  useEffect(() => {
    // Keep active key valid as the schema loads / changes.
    if (!entries.find((e) => e.key === activeKey) && entries.length > 0) {
      setActiveKey(entries[0].key)
    }
  }, [entries, activeKey])

  const active: SidebarEntry | undefined = entries.find((e) => e.key === activeKey)

  return (
    <div className="config-form">
      <nav className="config-form-sidebar" aria-label="配置分组">
        {entries.map((e) => {
          const isActive = e.key === activeKey
          const errorCount = e.kind === 'group' ? e.errorCount : errorCountForClients()
          return (
            <button
              type="button"
              key={e.key}
              className={`config-group-card${isActive ? ' active' : ''}`}
              onClick={() => setActiveKey(e.key)}
              aria-current={isActive ? 'true' : undefined}
            >
              <span className="config-group-card-label">
                {e.kind === 'group' ? e.group.label : '客户端'}
              </span>
              <span className="config-group-card-meta">
                {errorCount > 0 ? (
                  <span className="badge badge-err">{errorCount}</span>
                ) : null}
                <ChevronRight size={16} strokeWidth={1.75} className="config-group-card-chevron" />
              </span>
            </button>
          )
        })}
      </nav>

      <div className="config-form-body">
        {active?.kind === 'group' ? (
          <ConfigSectionCard
            group={active.group}
            values={values}
            onFieldChange={onFieldChange}
            errorByPath={errorByPath}
          />
        ) : null}
        {active?.kind === 'clients' ? (
          <ClientsSection
            values={values}
            onFieldChange={onFieldChange}
            errorByPath={errorByPath}
          />
        ) : null}
      </div>
    </div>
  )
}
