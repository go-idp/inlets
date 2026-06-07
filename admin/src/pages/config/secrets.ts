// Pure helpers for the config page secret handling.
// Secrets are masked on the wire (the API returns "***") but the page keeps
// the real values in a ref so that re-serializing the form doesn't wipe them.

import type { ConfigSchema, FieldDef } from '../../api/client'
import { getByPath } from '../../schema/renderers'

export const SECRET_PLACEHOLDER = '***'

export function maskSecretValue(field: FieldDef, v: unknown): unknown {
  if (field.kind === 'secret' && v && v !== SECRET_PLACEHOLDER) {
    return SECRET_PLACEHOLDER
  }
  return v
}

export function collectSecrets(values: any, schema: ConfigSchema | null): Record<string, string> {
  if (!schema || !values) return {}
  const out: Record<string, string> = {}
  for (const g of schema.groups) {
    for (const f of g.fields) {
      if (f.kind !== 'secret') continue
      const v = getByPath(values, f.path)
      if (typeof v === 'string' && v !== '' && v !== SECRET_PLACEHOLDER) {
        out[f.path] = v
      }
    }
  }
  if (Array.isArray(values?.clients)) {
    values.clients.forEach((c: any, i: number) => {
      if (c && typeof c.clientSecret === 'string' && c.clientSecret !== '' && c.clientSecret !== SECRET_PLACEHOLDER) {
        out[`clients[${i}].clientSecret`] = c.clientSecret
      }
    })
  }
  return out
}
