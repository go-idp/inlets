type ApiEnvelope<T> = {
  ok: boolean
  data?: T
  error?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (!headers.has('Content-Type') && init?.body) {
    headers.set('Content-Type', 'application/json')
  }
  const res = await fetch(`/api/v1${path}`, { ...init, headers })
  const body = (await res.json()) as ApiEnvelope<T>
  if (!res.ok || !body.ok) {
    throw new Error(body.error || res.statusText)
  }
  return body.data as T
}

export type AdminStatusData = {
  enabled: boolean
  listen?: string
  uiBasePath?: string
  databasePath?: string
  snapshotInterval?: string
  pidFile?: string
}

export type StatusData = {
  version: string
  configPath: string
  reloadReady: boolean
  domain: string
  httpPort: number
  tcpPort: number
  sessionCount: number
  admin?: AdminStatusData
}

export type OverviewData = {
  version: string
  domain: string
  httpPort: number
  tcpPort: number
  secure: boolean
  uptimeSeconds: number
  startedAt: string
  sessionCount: number
  domainCount: number
  stats: TrafficStats | null
}

export type SessionRow = {
  containerId: string
  clientId: string
  type: string
  authType: string
  version: string
  publicEntry: string
  sourcePort?: number
  useNewProtocol: boolean
  configIndex?: number
  configMatch?: '' | 'exact' | 'partial' | 'missing'
  matchIssues?: string[]
}

export type DomainRow = {
  subDomain: string
  clientId: string
}

export type TrafficStats = {
  uploadBytes: number
  downloadBytes: number
  requests: number
  connections: number
}

export type StatsData = {
  global: TrafficStats | null
  byClient: Record<string, TrafficStats>
}

export type MetricSnapshot = {
  id: number
  uploadBytes: number
  downloadBytes: number
  requests: number
  connections: number
  sessionCount: number
  createdAt: string
}

export type ConfigDocument = {
  path: string
  config: unknown
}

export type FieldKind =
  | 'string'
  | 'int'
  | 'port'
  | 'bool'
  | 'enum'
  | 'duration'
  | 'secret'

export type GroupKind = 'object' | 'list' | 'kvMap'

export type FieldDef = {
  path: string
  label: string
  kind: FieldKind
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

export type GroupDef = {
  key: string
  label: string
  path: string
  kind: GroupKind
  fields: FieldDef[]
}

export type ConfigSchema = {
  schemaVersion: number
  groups: GroupDef[]
}

export type ValidationError = { path: string; message: string }
export type ValidationResult = { ok: boolean; errors: ValidationError[] }

export type ConfigRaw = {
  path: string
  yaml: string
}

export type AuditRow = {
  id: number
  action: string
  summary: string
  actor: string
  clientIp: string
  createdAt: string
  diff?: string
}

export type RevisionRow = {
  id: number
  createdAt: string
  actor: string
  clientIp: string
  summary: string
  bytesSize: number
}

export type RevisionDetail = RevisionRow & {
  yaml: string
}

export type PutConfigResult = {
  reloaded: boolean
  revisionId?: number
}

export const api = {
  status: () => request<StatusData>('/status'),
  overview: () => request<OverviewData>('/overview'),
  sessions: () => request<SessionRow[]>('/sessions'),
  domains: () => request<DomainRow[]>('/domains'),
  stats: () => request<StatsData>('/stats'),
  statsHistory: (limit = 48) =>
    request<MetricSnapshot[]>(`/stats/history?limit=${limit}`),
  getConfig: (maskSecrets: boolean = true) =>
    request<ConfigDocument>(`/config?maskSecrets=${maskSecrets ? 'true' : 'false'}`),
  getConfigRaw: () => request<ConfigRaw>('/config/raw'),
  getConfigSchema: () => request<ConfigSchema>('/config/schema'),
  validateConfig: (yaml: string) =>
    request<ValidationResult>('/config/validate', {
      method: 'POST',
      body: JSON.stringify({ yaml }),
    }),
  putConfig: (yaml: string, summary?: string) =>
    request<PutConfigResult>('/config', {
      method: 'PUT',
      body: JSON.stringify({ yaml, summary: summary ?? '' }),
    }),
  listRevisions: (limit = 20) =>
    request<RevisionRow[]>(`/config/revisions?limit=${limit}`),
  getRevision: (id: number) =>
    request<RevisionDetail>(`/config/revisions/${id}`),
  restoreRevision: (id: number, summary?: string) =>
    request<PutConfigResult>(`/config/revisions/${id}/restore`, {
      method: 'POST',
      body: JSON.stringify({ summary: summary ?? '' }),
    }),
  reload: () =>
    request<{ reloaded: boolean }>('/reload', { method: 'POST' }),
  audit: (limit = 50) => request<AuditRow[]>(`/audit?limit=${limit}`),
}
