export type ConfigTabKey = 'visual' | 'yaml' | 'revisions'

type Props = {
  tab: ConfigTabKey
  onChange: (tab: ConfigTabKey) => void
}

const TABS: { key: ConfigTabKey; label: string }[] = [
  { key: 'visual', label: '可视化' },
  { key: 'yaml', label: 'YAML 源' },
  { key: 'revisions', label: '历史版本' },
]

export function ConfigTabs({ tab, onChange }: Props) {
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
          </button>
        )
      })}
    </div>
  )
}
