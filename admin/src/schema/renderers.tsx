import { useState } from 'react'
import type { FieldDef, ValidationError } from './types'

/** Read a value at a dotted/indexed path from an object. */
export function getByPath(obj: unknown, path: string): unknown {
  if (!path) return obj
  const segs = parsePath(path)
  let cur: any = obj
  for (const s of segs) {
    if (cur == null) return undefined
    cur = cur[s]
  }
  return cur
}

/** Set a value at a dotted/indexed path on an object (immutable). */
export function setByPath(obj: unknown, path: string, value: unknown): unknown {
  if (!path) return value
  const segs = parsePath(path)
  const root = Array.isArray(obj) ? [...obj] : { ...(obj as any) }
  let cur: any = root
  for (let i = 0; i < segs.length - 1; i++) {
    const seg = segs[i]
    const next = segs[i + 1]
    const isIndex = /^\d+$/.test(next)
    const existing = cur[seg]
    if (existing == null) {
      cur[seg] = isIndex ? [] : {}
    } else {
      cur[seg] = Array.isArray(existing) ? [...existing] : { ...existing }
    }
    cur = cur[seg]
  }
  cur[segs[segs.length - 1]] = value
  return root
}

function parsePath(path: string): string[] {
  // e.g. "clients[2].tunnels[0].upstream" -> ["clients","2","tunnels","0","upstream"]
  return path
    .replace(/\[(\d+)\]/g, '.$1')
    .split('.')
    .filter(Boolean)
}

interface BaseFieldProps {
  field: FieldDef
  value: unknown
  onChange: (v: unknown) => void
  error?: ValidationError
  secretStored?: boolean
}

export function FieldRenderer({ field, value, onChange, error, secretStored }: BaseFieldProps) {
  const id = `f-${field.path}`
  return (
    <div className={`config-field${error ? ' error' : ''}`}>
      <label htmlFor={id}>
        {field.label}
        {field.required ? <span className="req">*</span> : null}
      </label>
      <FieldInput field={field} value={value} onChange={onChange} id={id} secretStored={secretStored} />
      {error ? <span className="err-msg">{error.message}</span> : null}
      {!error && field.helpText ? <span className="help">{field.helpText}</span> : null}
    </div>
  )
}

function FieldInput({ field, value, onChange, id, secretStored }: BaseFieldProps & { id: string }) {
  switch (field.kind) {
    case 'bool':
      return (
        <select
          id={id}
          value={value === true || value === 'true' ? 'true' : value === false || value === 'false' ? 'false' : ''}
          onChange={(e) => onChange(e.target.value === 'true')}
        >
          <option value="">未设置</option>
          <option value="true">是</option>
          <option value="false">否</option>
        </select>
      )

    case 'enum':
      return (
        <select
          id={id}
          value={value == null ? '' : String(value)}
          onChange={(e) => onChange(e.target.value)}
        >
          <option value=""></option>
          {(field.enumValues || []).map((v) => (
            <option key={v} value={v}>{v}</option>
          ))}
        </select>
      )

    case 'int':
    case 'port':
      return (
        <input
          id={id}
          type="number"
          value={value == null ? '' : String(value)}
          min={field.min}
          max={field.max}
          placeholder={field.placeholder}
          onChange={(e) => {
            const s = e.target.value
            if (s === '') return onChange(null)
            const n = Number(s)
            onChange(Number.isFinite(n) ? n : s)
          }}
        />
      )

    case 'secret':
      return (
        <SecretInput
          id={id}
          value={value}
          onChange={onChange}
          placeholder={field.placeholder}
          stored={secretStored}
        />
      )

    case 'duration':
      return (
        <input
          id={id}
          type="text"
          value={value == null ? '' : String(value)}
          placeholder={field.placeholder || '5m / 30m / 1h'}
          onChange={(e) => onChange(e.target.value)}
        />
      )

    case 'string':
    default:
      return (
        <input
          id={id}
          type="text"
          value={value == null ? '' : String(value)}
          placeholder={field.placeholder}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }
}

function SecretInput({
  id, value, onChange, placeholder, stored,
}: {
  id: string
  value: unknown
  onChange: (v: unknown) => void
  placeholder?: string
  stored?: boolean
}) {
  const [shown, setShown] = useState(false)
  return (
    <div className="config-secret">
      <input
        id={id}
        type={shown ? 'text' : 'password'}
        value={value == null ? '' : String(value)}
        placeholder={placeholder ?? (stored ? '已设置' : undefined)}
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
      />
      {stored ? (
        <button type="button" onClick={() => onChange('')}>
          清除
        </button>
      ) : null}
      <button type="button" onClick={() => setShown((s) => !s)}>
        {shown ? '隐藏' : '显示'}
      </button>
    </div>
  )
}

/**
 * CardList renders a `list` group. It expects:
 * - field.value to be an array
 * - field.item to be a FieldDef describing each entry's shape (in this minimal
 *   version we treat the entry as a flat object edited by path suffixes).
 */
export function CardListEditor({
  value,
  itemLabel,
  onChange,
  renderItem,
}: {
  value: unknown[]
  itemLabel: (item: any, idx: number) => string
  onChange: (next: unknown[]) => void
  renderItem: (item: any, idx: number) => React.ReactNode
}) {
  return (
    <div className="config-card-list">
      {value.map((item, idx) => (
        <div className="config-card" key={idx}>
          <div className="config-card-head">
            <strong>{itemLabel(item, idx)}</strong>
            <div>
              <button
                type="button"
                onClick={() => {
                  if (idx === 0) return
                  const next = [...value]
                  ;[next[idx - 1], next[idx]] = [next[idx], next[idx - 1]]
                  onChange(next)
                }}
                title="上移"
              >
                ↑
              </button>
              <button
                type="button"
                onClick={() => {
                  if (idx === value.length - 1) return
                  const next = [...value]
                  ;[next[idx + 1], next[idx]] = [next[idx], next[idx + 1]]
                  onChange(next)
                }}
                title="下移"
              >
                ↓
              </button>
              <button
                type="button"
                onClick={() => {
                  const next = value.filter((_, i) => i !== idx)
                  onChange(next)
                }}
                title="删除"
              >
                ×
              </button>
            </div>
          </div>
          {renderItem(item, idx)}
        </div>
      ))}
      <button
        type="button"
        className="btn"
        onClick={() => onChange([...value, {}])}
      >
        + 新增
      </button>
    </div>
  )
}
