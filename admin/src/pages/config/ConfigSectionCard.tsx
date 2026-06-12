import type { GroupDef, FieldDef, ValidationError } from '../../api/client'
import { FieldRenderer, getByPath } from '../../schema/renderers'
import { hasStoredSecret, secretDisplayValue } from './secrets'

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
        const raw = getByPath(values, f.path)
        const isSecret = f.kind === 'secret'
        return (
          <FieldRenderer
            key={f.path}
            field={f}
            value={isSecret ? secretDisplayValue(raw) : raw}
            secretStored={isSecret && hasStoredSecret(raw)}
            onChange={(nv) => onFieldChange(f, nv)}
            error={errorByPath[f.path]}
          />
        )
      })}
    </div>
  )
}
