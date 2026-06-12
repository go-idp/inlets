import YAML from 'yaml'

/**
 * Serialize a structured values object back to YAML.
 *
 * The roundtrip property we rely on is:
 *   YAML.parse(serialize(values)) deep-equals YAML.parse(yamlString)
 * for any value derived from the original document by mutating only leaf
 * fields reachable through FieldDef.path.
 *
 * Notes / non-goals:
 * - Comments in the original YAML are NOT preserved (YAML library limitation).
 *   This is documented behavior accepted by the user in the PR-1 plan.
 * - We DO preserve field ordering by walking the parsed object and emitting
 *   fields in the order they first appear in the source YAML.
 */
export function serializeValuesToYAML(values: unknown, sourceYAML?: string): string {
  const sanitized = sanitizeForSave(values)
  const ordered = orderFromSource(sanitized, sourceYAML)
  return YAML.stringify(ordered, {
    lineWidth: 0,
    sortMapEntries: false,
  })
}

/** Drop v1-only client keys and empty tunnel fields before persisting. */
function sanitizeForSave(values: unknown): unknown {
  if (values == null || typeof values !== 'object' || Array.isArray(values)) {
    return values
  }
  const root = { ...(values as Record<string, unknown>) }
  if (Array.isArray(root.clients)) {
    root.clients = root.clients.map(sanitizeClientEntry)
  }
  if (root.token === '' || root.token == null) {
    delete root.token
  }
  return root
}

const legacyClientKeys = new Set(['type', 'port'])

function sanitizeClientEntry(client: unknown): unknown {
  if (client == null || typeof client !== 'object' || Array.isArray(client)) {
    return client
  }
  const next: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(client as Record<string, unknown>)) {
    if (legacyClientKeys.has(k)) continue
    if (k === 'tunnels' && Array.isArray(v)) {
      next.tunnels = v.map(sanitizeTunnelEntry)
    } else {
      next[k] = v
    }
  }
  return next
}

function sanitizeTunnelEntry(tunnel: unknown): unknown {
  if (tunnel == null || typeof tunnel !== 'object' || Array.isArray(tunnel)) {
    return tunnel
  }
  const src = tunnel as Record<string, unknown>
  const type = String(src.type ?? '').toLowerCase()
  const next: Record<string, unknown> = { ...src }
  if (type === 'http') {
    delete next.remotePort
  } else if (type === 'tcp') {
    delete next.subDomain
  }
  for (const k of ['name', 'subDomain'] as const) {
    const v = next[k]
    if (v == null || (typeof v === 'string' && !v.trim())) {
      delete next[k]
    }
  }
  if (next.remotePort === '' || next.remotePort == null) {
    delete next.remotePort
  }
  return next
}

function orderFromSource(values: unknown, sourceYAML?: string): unknown {
  if (!sourceYAML) return values
  let parsed: any
  try {
    parsed = YAML.parse(sourceYAML)
  } catch {
    return values
  }
  return reorderInPlace(values, parsed)
}

function reorderInPlace(target: unknown, template: unknown): unknown {
  if (target == null || template == null) return target
  if (Array.isArray(template)) {
    if (!Array.isArray(target)) return target
    return target.map((t, i) => reorderInPlace(t, (template as unknown[])[i] ?? {}))
  }
  if (typeof template !== 'object' || typeof target !== 'object') return target
  const out: Record<string, unknown> = {}
  for (const k of Object.keys(template)) {
    if (k in (target as any)) {
      out[k] = reorderInPlace((target as any)[k], (template as any)[k])
    }
  }
  // Carry over extra keys the user added that weren't in the template.
  for (const k of Object.keys(target as any)) {
    if (!(k in out)) out[k] = (target as any)[k]
  }
  return out
}
