import type { ValidationError } from '../../api/client'
import { FieldRenderer } from '../../schema/renderers'

export type TunnelRecord = {
  name?: string
  type?: string
  upstream?: string
  subDomain?: string
  remotePort?: number | string | null
}

type Props = {
  base: string
  tunnelIndex: number
  item: TunnelRecord
  onChange: (item: TunnelRecord) => void
  errorByPath: Record<string, ValidationError>
}

export function TunnelForm({ base, tunnelIndex, item, onChange, errorByPath }: Props) {
  const prefix = `${base}.tunnels[${tunnelIndex}]`

  const update = (k: keyof TunnelRecord, v: unknown) => {
    onChange({ ...(item ?? {}), [k]: v })
  }

  return (
    <div>
      <FieldRenderer
        field={{ path: `${prefix}.name`, label: '名称', kind: 'string' }}
        value={item?.name ?? ''}
        onChange={(v) => update('name', v)}
      />
      <FieldRenderer
        field={{ path: `${prefix}.type`, label: '类型', kind: 'enum', required: true, enumValues: ['http', 'tcp'] }}
        value={item?.type ?? ''}
        onChange={(v) => update('type', v)}
        error={errorByPath[`${prefix}.type`]}
      />
      <FieldRenderer
        field={{ path: `${prefix}.upstream`, label: '上游', kind: 'string', required: true, placeholder: '127.0.0.1:8080' }}
        value={item?.upstream ?? ''}
        onChange={(v) => update('upstream', v)}
        error={errorByPath[`${prefix}.upstream`]}
      />
      <FieldRenderer
        field={{ path: `${prefix}.subDomain`, label: '子域', kind: 'string', helpText: 'HTTP 隧道专属' }}
        value={item?.subDomain ?? ''}
        onChange={(v) => update('subDomain', v)}
      />
      <FieldRenderer
        field={{ path: `${prefix}.remotePort`, label: '远程端口', kind: 'port', helpText: 'TCP 隧道专属；0 = 沿用客户端 -p' }}
        value={item?.remotePort ?? ''}
        onChange={(v) => update('remotePort', v)}
      />
    </div>
  )
}

function emptyTunnel(): TunnelRecord {
  return { name: '', type: 'http', upstream: '', subDomain: '', remotePort: '' }
}

export { emptyTunnel }
