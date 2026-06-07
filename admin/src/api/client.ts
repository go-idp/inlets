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

export type StatusData = {
  version: string
  configPath: string
  reloadReady: boolean
  domain: string
  httpPort: number
  tcpPort: number
  sessionCount: number
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
}

export const api = {
  status: () => request<StatusData>('/status'),
  overview: () => request<OverviewData>('/overview'),
  sessions: () => request<SessionRow[]>('/sessions'),
  domains: () => request<DomainRow[]>('/domains'),
  stats: () => request<StatsData>('/stats'),
  statsHistory: (limit = 48) =>
    request<MetricSnapshot[]>(`/stats/history?limit=${limit}`),
  getConfig: (maskSecrets = true) =>
    request<ConfigDocument>(`/config?maskSecrets=${maskSecrets ? 'true' : 'false'}`),
  getConfigRaw: () => request<ConfigRaw>('/config/raw'),
  putConfig: (yaml: string) =>
    request<{ reloaded: boolean }>('/config', {
      method: 'PUT',
      body: JSON.stringify({ yaml }),
    }),
  reload: () =>
    request<{ reloaded: boolean }>('/reload', { method: 'POST' }),
  audit: (limit = 50) => request<AuditRow[]>(`/audit?limit=${limit}`),
}
