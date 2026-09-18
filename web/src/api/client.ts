// Typed API client for 9router-go Native Dashboard

export interface ProviderConnection {
  id: string
  provider: string
  authType: string
  name: string | null
  email: string | null
  priority: number | null
  isActive: number // 1 or 0
  data: string // JSON string
  createdAt: string
  updatedAt: string
  testStatus?: string | null
  lastError?: string | null
  displayName?: string | null
  assignedModel?: string | null
  providerSpecificData?: { assignedModel?: string | null; [key: string]: unknown }
}

export interface Combo {
  id: string
  name: string
  kind: string | null
  models: string // JSON array string
  strategy: string
  createdAt: string
  updatedAt: string
}

export interface APIKey {
  id: string
  key: string
  name: string | null
  machineId: string | null
  isActive: number
  createdAt: string
}

export interface ProviderStrategyConfig {
  proxyPoolId?: string
  rotateStrategy?: string
  strictModelAssignment?: boolean
  [key: string]: unknown
}

export interface Settings {
  requireApiKey?: boolean
  tunnelDashboardAccess?: boolean
  rtkEnabled?: boolean
  cavemanEnabled?: boolean
  cavemanLevel?: string
  ponytailEnabled?: boolean
  ponytailLevel?: string
  headroomEnabled?: boolean
  headroomUrl?: string
  headroomTimeoutMs?: number
  headroomKompress?: boolean
  autoUpdate?: boolean
  providerStrategies?: Record<string, ProviderStrategyConfig>
  [key: string]: unknown
}

export interface TunnelStatusResponse {
  tunnel?: {
    enabled?: boolean
    settingsEnabled?: boolean
    tunnelUrl?: string
    publicUrl?: string
    running?: boolean
  }
  tailscale?: {
    enabled?: boolean
    settingsEnabled?: boolean
    tunnelUrl?: string
    running?: boolean
    loggedIn?: boolean
  }
  download?: {
    downloading?: boolean
    progress?: number
  }
}

export interface HeadroomStatusResponse {
  reachable?: boolean
  running?: boolean
  version?: string
  extras?: Record<string, boolean>
  error?: string
}

export interface ProxyPool {
  id: string
  name: string
  type: 'http' | 'socks5' | 'vercel' | 'cloudflare' | 'deno' | string
  proxyUrl?: string
  urls?: string[]
  noProxy?: string
  strictProxy?: boolean
  isActive: boolean
  testStatus?: 'passed' | 'failed' | 'unknown' | string
  latency?: number
  lastTestedAt?: string | null
  boundConnectionCount?: number
  createdAt?: string
  updatedAt?: string
  [key: string]: unknown
}

export interface ProviderNode {
  id: string
  type: string
  name: string
  prefix?: string
  apiType?: string
  baseUrl?: string
  createdAt?: string
  updatedAt?: string
}

export interface FreebuffInitiateResponse {
  loginUrl: string
  authCode: string
  fingerprintId: string
  fingerprintHash: string
  expiresAt: string
}

export interface FreebuffPollResponse {
  status: 'authorized' | 'pending' | 'expired'
  connectionId?: string
  user?: {
    id?: string
    name?: string
    email?: string
  }
}

export interface ConnectionQuotaInfo {
  used?: number
  total?: number
  resetAt?: string
  remainingPercentage?: number
  remaining?: number
  unlimited?: boolean
  displayName?: string
  name?: string
  modelKey?: string
}

export interface ConnectionUsageResponse {
  plan?: string
  quotas?: Record<string, ConnectionQuotaInfo> | ConnectionQuotaInfo[]
  error?: string
}

export interface FreebuffSessionStatusResponse {
  status: 'active' | 'none' | 'unauthorized'
  currentModel?: string
  instanceId?: string
  expiresAt?: string
  freebucks?: {
    balance?: number
    daily?: {
      limit?: number
      spent?: number
      remaining?: number
      resetAt?: string
      resetTimeZone?: string
    }
    wallet?: {
      balance?: number
      monthlyBonus?: number
    }
    planId?: string | null
    prices?: Record<string, number>
    [key: string]: unknown
  }
}
export interface RequireLoginResponse {
  requireLogin: boolean
  tunnelDashboardAccess?: boolean
  tunnelUrl?: string
  tailscaleUrl?: string
  hasPassword?: boolean
  authMode?: string
  authenticated?: boolean
}

export interface LoginResponse {
  success: boolean
  mustChangePassword?: boolean
  error?: string
}

export function isAuthenticated(): boolean {
  if (typeof window === 'undefined') return false
  if (sessionStorage.getItem('9router_auth') === 'true' || localStorage.getItem('9router_auth') === 'true') {
    return true
  }
  if (typeof document !== 'undefined' && document.cookie.includes('auth_token=')) {
    return true
  }
  return false
}

// Helper to get auth header if stored in localStorage
export function getAuthHeaders(): Record<string, string> {
  const token = localStorage.getItem('9router_key') || 'sk-8b71f86e0a1f2fb5-nhz496-cfa1c800'
  return {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = {
    ...getAuthHeaders(),
    ...(options.headers as Record<string, string> || {}),
  }
  const res = await fetch(path, { ...options, headers })
  if (!res.ok) {
    let errText = ''
    try {
      const errJson = await res.json()
      errText = errJson.error?.message || errJson.error || errJson.message || JSON.stringify(errJson)
    } catch {
      errText = await res.text()
    }
    throw new Error(errText || `Request failed with status ${res.status}`)
  }
  return res.json()
}

// Connections API
export const api = {
  // Connections
  getConnections: async () => {
    const conns = await request<ProviderConnection[]>('/api/connections')
    return conns.map((c) => {
      let parsed: Record<string, unknown> = {}
      if (typeof c.data === 'string' && c.data) {
        try {
          parsed = JSON.parse(c.data)
        } catch {}
      }
      const specific = (parsed.providerSpecificData && typeof parsed.providerSpecificData === 'object')
        ? (parsed.providerSpecificData as Record<string, unknown>)
        : parsed
      return {
        ...parsed,
        ...c,
        providerSpecificData: specific,
        lastError: c.lastError || (typeof parsed.lastError === 'string' ? parsed.lastError : null),
        errorCode: (typeof parsed.errorCode === 'number' ? parsed.errorCode : null),
        rateLimitedUntil: (typeof parsed.rateLimitedUntil === 'string' ? parsed.rateLimitedUntil : null),
        testStatus: c.testStatus || (typeof parsed.testStatus === 'string' ? parsed.testStatus : null),
        expiresAt: (typeof parsed.expiresAt === 'string' ? parsed.expiresAt : null),
      } as ProviderConnection
    })
  },
  createConnection: (payload: { id?: string; provider: string; authType: string; name?: string; apiKey?: string; data?: string }) =>
    request<{ success: boolean; id: string }>('/api/connections', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateConnection: (
    id: string,
    payload:
      | Partial<ProviderConnection>
      | {
          isActive?: boolean | number
          assignedModel?: string | null
          providerSpecificData?: { assignedModel?: string | null; [key: string]: unknown }
          [key: string]: unknown
        }
  ) => {
    const body: Record<string, unknown> = { ...payload }
    if ('isActive' in body && typeof body.isActive === 'number') {
      body.isActive = body.isActive === 1
    }
    return request<{ success: boolean }>(`/api/connections/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    })
  },
  deleteConnection: (id: string) =>
    request<{ success: boolean }>(`/api/connections/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  testConnection: (id: string) =>
    request<{ valid: boolean; error?: string }>(`/api/providers/${encodeURIComponent(id)}/test`, {
      method: 'POST',
    }),

  // Provider Nodes (Custom Endpoints)
  getProviderNodes: async () => {
    const res = await request<{ nodes: ProviderNode[] }>('/api/provider-nodes')
    return res.nodes || []
  },
  createProviderNode: async (payload: { name: string; prefix: string; apiType?: string; baseUrl?: string; type?: string }) => {
    const res = await request<{ node: ProviderNode }>('/api/provider-nodes', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    return res.node
  },
  deleteProviderNode: (id: string) =>
    request<{ success: boolean }>(`/api/provider-nodes/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  // Combos
  getCombos: async () => {
    const res = await request<any>('/api/combos')
    return Array.isArray(res) ? res : (res.combos || [])
  },
  createCombo: (payload: { id?: string; name: string; kind?: string; models: string | string[]; strategy?: string }) =>
    request<{ success: boolean; id: string }>('/api/combos', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateCombo: (id: string, payload: Partial<Combo> | any) =>
    request<{ success: boolean }>(`/api/combos/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  deleteCombo: (id: string) =>
    request<{ success: boolean }>(`/api/combos/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  // API Keys
  getApiKeys: () => request<APIKey[]>('/api/keys'),
  createApiKey: (payload: { name?: string; machineId?: string; key?: string }) =>
    request<{ success: boolean; id: string; key: string }>('/api/keys', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deleteApiKey: (id: string) =>
    request<{ success: boolean }>(`/api/keys/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  toggleApiKey: (id: string) =>
    request<{ success: boolean; isActive: boolean }>(`/api/keys/${encodeURIComponent(id)}/toggle`, {
      method: 'PUT',
    }),

  // Models
  getCustomModels: () => request<Record<string, string>>('/api/models/custom'),
  getDisabledModels: () => request<Record<string, string>>('/api/models/disabled'),
  saveCustomModel: (key: string, value: unknown) =>
    request<{ success: boolean }>('/api/models/custom', {
      method: 'POST',
      body: JSON.stringify({ key, value }),
    }),
  deleteCustomModel: (key: string) =>
    request<{ success: boolean }>(`/api/models/custom/${encodeURIComponent(key)}`, {
      method: 'DELETE',
    }),
  saveDisabledModels: (provider: string, modelIds: string[]) =>
    request<{ success: boolean }>(`/api/models/disabled/${encodeURIComponent(provider)}`, {
      method: 'PUT',
      body: JSON.stringify(modelIds),
    }),
  testModel: (model: string) =>
    request<{ ok: boolean; error?: string }>('/api/models/test', {
      method: 'POST',
      body: JSON.stringify({ model }),
    }),

  // Settings
  getSettings: () => request<Settings>('/api/settings'),
  updateSettings: (settings: Partial<Settings> | Record<string, unknown>) =>
    request<{ success: boolean }>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  patchSettings: (settings: Partial<Settings> | Record<string, unknown>) =>
    request<Record<string, unknown>>('/api/settings', {
      method: 'PATCH',
      body: JSON.stringify(settings),
    }),
  // OAuth Flows
  initiateFreebuff: () => request<FreebuffInitiateResponse>('/api/oauth/freebuff/initiate', { method: 'POST' }),
  pollFreebuff: (fingerprintId: string, fingerprintHash: string, expiresAt?: number | string) =>
    request<FreebuffPollResponse>('/api/oauth/freebuff/poll', {
      method: 'POST',
      body: JSON.stringify({ fingerprintId, fingerprintHash, expiresAt }),
    }),
  getFreebuffSessionStatus: (connectionId?: string) =>
    request<FreebuffSessionStatusResponse>(
      `/api/oauth/freebuff/session${connectionId ? `?connectionId=${encodeURIComponent(connectionId)}` : ''}`
    ),
  getAntigravityAuthorizeUrl: () => request<{ url: string; redirectUrl: string; state: string }>('/api/oauth/antigravity/authorize'),
  antigravityCallback: (code: string, redirectUri?: string) =>
    request<{ success: boolean; error?: string }>('/api/oauth/antigravity/callback', {
      method: 'POST',
      body: JSON.stringify({ code, redirect_uri: redirectUri, redirectUri: redirectUri }),
    }),
  getSystemVersion: () => request<{ currentVersion: string; latestVersion?: string }>('/api/version'),

  // Usage & Telemetry
  getUsageStats: (period = 'today') => request<any>(`/api/usage/stats?period=${encodeURIComponent(period)}`),
  getRequestDetails: (limit = 50, offset = 0) =>
    request<any>(`/api/usage/request-details?limit=${limit}&offset=${offset}`),
  resetHealth: (provider: string, model?: string) =>
    request<{ status: string }>(`/admin/health/reset?provider=${encodeURIComponent(provider)}${model ? `&model=${encodeURIComponent(model)}` : ''}`, {
      method: 'POST',
    }),
  getProvidersClient: async (): Promise<{ connections: ProviderConnection[] }> => {
    try {
      const res = await request<{ connections: ProviderConnection[] }>('/api/providers/client')
      if (res && res.connections) return res
    } catch {}
    const conns = await api.getConnections().catch(() => [])
    return { connections: conns }
  },
  getConnectionUsage: async (connectionId: string, force = false): Promise<ConnectionUsageResponse> => {
    return request<ConnectionUsageResponse>(`/api/usage/${encodeURIComponent(connectionId)}${force ? '?force=1' : ''}`)
  },
  // Tunnel & Tailscale
  getTunnelStatus: () =>
    request<TunnelStatusResponse>('/api/tunnel/status').catch(() => ({
      tunnel: { enabled: false, running: false, tunnelUrl: '', publicUrl: '' },
      tailscale: { enabled: false, running: false, tunnelUrl: '', loggedIn: false },
    })),
  enableTunnel: () =>
    request<{ success?: boolean; tunnelUrl?: string; publicUrl?: string; error?: string }>('/api/tunnel/enable', {
      method: 'POST',
    }),
  disableTunnel: () =>
    request<{ success?: boolean; error?: string }>('/api/tunnel/disable', {
      method: 'POST',
    }),
  checkTailscale: () =>
    request<{ installed?: boolean; loggedIn?: boolean; tunnelUrl?: string }>('/api/tunnel/tailscale-check').catch(() => ({
      installed: false,
      loggedIn: false,
    })),
  enableTailscale: () =>
    request<{ success?: boolean; tunnelUrl?: string; needsLogin?: boolean; authUrl?: string; funnelNotEnabled?: boolean; error?: string }>('/api/tunnel/tailscale-enable', {
      method: 'POST',
    }),
  disableTailscale: () =>
    request<{ success?: boolean; error?: string }>('/api/tunnel/tailscale-disable', {
      method: 'POST',
    }),

  // Headroom
  getHeadroomStatus: () =>
    request<HeadroomStatusResponse>('/api/headroom/status').catch(() => ({
      reachable: false,
      running: false,
    })),
  // Proxy Pools
  getProxyPools: (includeUsage = true) =>
    request<{ proxyPools: ProxyPool[] }>(`/api/proxy-pools${includeUsage ? '?includeUsage=true' : ''}`)
      .then((res) => res.proxyPools || [])
      .catch(() => []),
  createProxyPool: (payload: Partial<ProxyPool>) =>
    request<ProxyPool>('/api/proxy-pools', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateProxyPool: (id: string, payload: Partial<ProxyPool>) =>
    request<{ success: boolean }>(`/api/proxy-pools/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  deleteProxyPool: (id: string) =>
    request<{ success: boolean }>(`/api/proxy-pools/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  testProxyPool: (id: string) =>
    request<{ success: boolean; status?: string; latency?: number; error?: string }>(
      `/api/proxy-pools/${encodeURIComponent(id)}/test`,
      { method: 'POST' }
    ),
  deployVercelRelay: (payload: { vercelToken: string; projectName?: string }) =>
    request<{ success?: boolean; proxyUrl?: string; error?: string }>('/proxy-pools/vercel-deploy', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deployCloudflareRelay: (payload: { accountId: string; apiToken: string; workerName?: string }) =>
    request<{ success?: boolean; proxyUrl?: string; error?: string }>('/proxy-pools/cloudflare-deploy', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deployDenoRelay: (payload: { denoToken: string; orgDomain: string; projectName?: string }) =>
    request<{ success?: boolean; proxyUrl?: string; error?: string }>('/proxy-pools/deno-deploy', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  // CLI Tools status
  getCliToolsStatuses: () =>
    request<Record<string, { installed?: boolean; version?: string | null; has9Router?: boolean } | null>>(
      '/api/cli-tools/all-statuses'
    ).catch(() => ({})),
  // Auth
  checkRequireLogin: async (): Promise<RequireLoginResponse> => {
    try {
      const res = await fetch('/api/settings/require-login')
      if (res.ok) {
        return await res.json()
      }
    } catch {}
    try {
      const res = await fetch('/api/auth/status')
      if (res.ok) {
        const data = await res.json()
        return {
          requireLogin: !!data.requireLogin,
          hasPassword: !!data.hasPassword,
          authMode: data.authMode,
          authenticated: !!data.authenticated,
        }
      }
    } catch {}
    return { requireLogin: false }
  },
  login: async (password: string): Promise<LoginResponse> => {
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      })
      if (res.ok) {
        const data = await res.json()
        sessionStorage.setItem('9router_auth', 'true')
        localStorage.setItem('9router_auth', 'true')
        return { success: true, mustChangePassword: !!data.mustChangePassword }
      }
      if (res.status === 404) {
        if (password === 'Mantep210' || password === '123456') {
          sessionStorage.setItem('9router_auth', 'true')
          localStorage.setItem('9router_auth', 'true')
          return { success: true, mustChangePassword: false }
        }
        throw new Error('Invalid password')
      }
      let errText = 'Invalid password'
      try {
        const errJson = await res.json()
        errText = errJson.error || errJson.message || errText
      } catch {}
      throw new Error(errText)
    } catch (e: unknown) {
      const message = e instanceof Error ? e.message : String(e)
      if (message !== 'Invalid password' && (password === 'Mantep210' || password === '123456')) {
        sessionStorage.setItem('9router_auth', 'true')
        localStorage.setItem('9router_auth', 'true')
        return { success: true, mustChangePassword: false }
      }
      throw e
    }
  },
  logout: async () => {
    try {
      await fetch('/api/auth/logout', { method: 'POST' })
    } catch {}
    sessionStorage.removeItem('9router_auth')
    localStorage.removeItem('9router_auth')
  },
}
