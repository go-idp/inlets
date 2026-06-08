import YAML from 'yaml'

/** Parse on-disk YAML into the object shape expected by schema field paths. */
export function valuesFromYAML(rawYaml: string): Record<string, unknown> {
  const trimmed = rawYaml.trim()
  if (!trimmed) return {}
  const parsed = YAML.parse(trimmed)
  if (parsed == null) return {}
  if (typeof parsed !== 'object' || Array.isArray(parsed)) return {}
  return parsed as Record<string, unknown>
}
