export type ConfigTabKey = 'visual' | 'yaml' | 'override' | 'revisions'

type Props = {
  tab: ConfigTabKey
  onChange: (tab: ConfigTabKey) => void
  overrideCount: number
}

const TABS: { key: ConfigTabKey; label: string }[] = [
  { key: 'visual', label: '可视化' },
  { key: 'yaml', label: 'YAML 源' },
  { key: 'override', label: '临时覆盖' },
  { key: 'revisions', label: '历史版本' },
]

export function ConfigTabs({ tab, onChange, overrideCount }: Props) {
  return (
    <div className="config-tabs" role="tablist">
      {TABS.map((t) => {
        const isActive = t.key === tab
        return (
          <button
            key={t.key}
            type="button"
            role="tab"
            aria-selected={isActive}
            className={isActive ? 'active' : ''}
            onClick={() => onChange(t.key)}
          >
            {t.label}
            {t.key === 'override' && overrideCount > 0 ? (
              <span className="config-tab-badge">{overrideCount}</span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
