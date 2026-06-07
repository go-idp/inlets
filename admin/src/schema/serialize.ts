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
  const ordered = orderFromSource(values, sourceYAML)
  return YAML.stringify(ordered, {
    lineWidth: 0,
    sortMapEntries: false,
  })
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
    return target.map((t) => reorderInPlace(t, template[0] ?? {}))
  }
  if (typeof template !== 'object' || typeof target !== 'object') return target
  const out: Record<string, unknown> = {}
  for (const k of Object.keys(template)) {
    if (k in (target as any)) {
      out[k] = reorderInPlace((target as any)[k], (template as any)[k])
    } else {
      out[k] = (template as any)[k]
    }
  }
  // Carry over extra keys the user added that weren't in the template.
  for (const k of Object.keys(target as any)) {
    if (!(k in out)) out[k] = (target as any)[k]
  }
  return out
}
