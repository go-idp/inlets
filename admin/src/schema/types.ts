export type SchemaFieldKind =
  | 'string'
  | 'int'
  | 'port'
  | 'bool'
  | 'enum'
  | 'duration'
  | 'secret'

export interface FieldDef {
  path: string
  label: string
  kind: SchemaFieldKind
  required?: boolean
  helpText?: string
  placeholder?: string
  min?: number
  max?: number
  enumValues?: string[]
  default?: unknown
  item?: FieldDef
  valueFields?: FieldDef[]
}

export type GroupKind = 'object' | 'list' | 'kvMap'

export interface GroupDef {
  key: string
  label: string
  path: string
  kind: GroupKind
  fields: FieldDef[]
}

export interface ConfigSchema {
  schemaVersion: number
  groups: GroupDef[]
}

export type ConfigValue = unknown

export interface ConfigDocument {
  path: string
  config: ConfigValue
}

/** A single validation problem reported by the server. */
export interface ValidationError {
  path: string
  message: string
}

export interface ValidationResult {
  ok: boolean
  errors: ValidationError[]
}
