import type { GroupDef, FieldDef, ValidationError } from '../../api/client'
import { FieldRenderer, getByPath } from '../../schema/renderers'
import { maskSecretValue } from './secrets'

type Props = {
  group: GroupDef
  values: any
  onFieldChange: (f: FieldDef, v: unknown) => void
  errorByPath: Record<string, ValidationError>
}

export function ConfigSectionCard({ group, values, onFieldChange, errorByPath }: Props) {
  return (
    <div className="config-section">
      <h3>{group.label}</h3>
      {group.fields.map((f) => {
        const v = getByPath(values, f.path)
        return (
          <FieldRenderer
            key={f.path}
            field={f}
            value={maskSecretValue(f, v)}
            onChange={(nv) => onFieldChange(f, nv)}
            error={errorByPath[f.path]}
          />
        )
      })}
    </div>
  )
}
