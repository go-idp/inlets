type Props = {
  value: string
  onChange: (v: string) => void
}

export function ConfigYamlEditor({ value, onChange }: Props) {
  return (
    <div className="panel">
      <textarea
        className="yaml-editor"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
    </div>
  )
}
