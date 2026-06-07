import type { ValidationError } from '../../api/client'
import { FieldRenderer } from '../../schema/renderers'
import { SECRET_PLACEHOLDER } from './secrets'

type Props = {
  base: string
  mode: 'create' | 'edit'
  item: any
  onChange: (item: any) => void
  errorByPath: Record<string, ValidationError>
}

export function ClientForm({ base, mode, item, onChange, errorByPath }: Props) {
  const updateField = (k: string, v: unknown) => {
    onChange({ ...(item ?? {}), [k]: v })
  }

  // Draft 里直接用原始值，不能 maskSecretValue —— 否则用户输入会被立刻改回 ***
  const secretValue = item?.clientSecret ?? ''
  const secretPlaceholder = mode === 'edit' ? '留空表示不修改' : undefined

  return (
    <div>
      <FieldRenderer
        field={{ path: `${base}.clientId`, label: 'Client ID', kind: 'string', required: true }}
        value={item?.clientId ?? ''}
        onChange={(v) => updateField('clientId', v)}
        error={errorByPath[`${base}.clientId`]}
      />
      <FieldRenderer
        field={{
          path: `${base}.clientSecret`,
          label: 'Client Secret',
          kind: 'secret',
          required: mode === 'create',
          placeholder: secretPlaceholder,
          helpText: mode === 'edit' ? '留空则保留原密钥' : undefined,
        }}
        value={secretValue === SECRET_PLACEHOLDER ? '' : secretValue}
        onChange={(v) => updateField('clientSecret', v)}
        error={errorByPath[`${base}.clientSecret`]}
      />
    </div>
  )
}
