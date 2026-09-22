// Port of 9router Next.js ProviderLimits/utils.js for the native Go dashboard.
// Pure helpers + localStorage quota cache + quota visibility filtering.

// ─── Constants ───────────────────────────────────────────────────────────────
export const QUOTA_CACHE_KEY = 'quotaCacheData'
export const REFRESH_INTERVAL_MS = 60000
// Claude usage/quota endpoint rate-limits; poll it less often than others
export const CLAUDE_REFRESH_INTERVAL_MS = 600000
export const DEPLETED_QUOTA_THRESHOLD = 5
export const AUTO_REFRESH_STORAGE_KEY = 'quotaAutoRefresh'
export const ACCOUNT_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
export const ACCOUNT_PAGE_SIZE_MAX = 500

export const ACCOUNT_FILTER_OPTIONS = [
  { value: 'all', label: 'All accounts' },
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Turned off' },
]

export const QUOTA_SORT_OPTIONS = [
  { value: 'default', label: 'Default quota order' },
  { value: 'remaining-asc', label: '% quota: low to high' },
  { value: 'remaining-desc', label: '% quota: high to low' },
]

// ─── Types ───────────────────────────────────────────────────────────────────
export interface NormalizedQuota {
  name: string
  modelKey?: string
  quotaType?: string
  used: number
  total: number
  remaining?: number
  remainingPercentage?: number
  resetAt?: string | null
  unlimited?: boolean
  recurring?: boolean
  isCreditBalance?: boolean
  currency?: string
  unit?: string
  message?: string
  [key: string]: unknown
}

export interface QuotaEntry {
  quotas: NormalizedQuota[]
  plan?: string | null
  message?: string | null
  raw?: unknown
  cachedAt?: string
}

export interface Pagination {
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export interface Totals {
  eligibleConnections: number
  providerFilteredConnections: number
}

export interface ProviderConnectionLike {
  id: string
  provider: string
  authType?: string
  name?: string | null
  email?: string | null
  displayName?: string | null
  isActive?: number | boolean
  testStatus?: string | null
  providerSpecificData?: { authMethod?: string; region?: string; profileArn?: string; [key: string]: unknown }
  [key: string]: unknown
}

// ─── Pure helpers ────────────────────────────────────────────────────────────
export function getConnectionLabel(connection: ProviderConnectionLike): string | null {
  return (
    connection.name?.trim() ||
    connection.email?.trim() ||
    connection.displayName?.trim() ||
    null
  )
}

// Stable group-by-provider: first-seen provider order, original order within group.
function groupByProviderStable<T extends ProviderConnectionLike>(connections: T[]): T[] {
  const seen = new Map<string, T[]>()
  for (const conn of connections) {
    const key = conn.provider || ''
    if (!seen.has(key)) seen.set(key, [])
    seen.get(key)!.push(conn)
  }
  return Array.from(seen.values()).flat()
}

export function getConnectionQuotaRemaining(
  connection: ProviderConnectionLike,
  quotaData: Record<string, QuotaEntry | undefined>,
): number {
  const quota = quotaData[connection.id]?.quotas?.[0]
  if (!quota) return Number.POSITIVE_INFINITY
  if (typeof quota.remaining === 'number') return quota.remaining
  return Number.POSITIVE_INFINITY
}

export function sortVisibleConnections<T extends ProviderConnectionLike>(
  connections: T[],
  quotaData: Record<string, QuotaEntry | undefined>,
  expiringFirst: boolean,
  providerFilter: string,
  quotaSortMode: string,
): T[] {
  if (providerFilter === 'codex' && quotaSortMode !== 'default') {
    return [...connections].sort((a, b) => {
      const remainingA = getConnectionQuotaRemaining(a, quotaData)
      const remainingB = getConnectionQuotaRemaining(b, quotaData)
      const remainingDiff =
        quotaSortMode === 'remaining-asc' ? remainingA - remainingB : remainingB - remainingA
      if (remainingDiff !== 0) return remainingDiff
      return (getConnectionLabel(a) || '').localeCompare(getConnectionLabel(b) || '')
    })
  }

  if (!expiringFirst) return groupByProviderStable(connections)

  const getEarliestResetTime = (connection: ProviderConnectionLike): number => {
    const resetTimes = (quotaData[connection.id]?.quotas || [])
      .map((quota) =>
        quota.resetAt ? new Date(quota.resetAt).getTime() : Number.POSITIVE_INFINITY,
      )
      .filter((time) => Number.isFinite(time))
    return resetTimes.length > 0 ? Math.min(...resetTimes) : Number.POSITIVE_INFINITY
  }

  return [...connections].sort((a, b) => {
    const expiryDiff = getEarliestResetTime(a) - getEarliestResetTime(b)
    if (expiryDiff !== 0) return expiryDiff
    return (
      (a.provider || '').localeCompare(b.provider || '') ||
      (getConnectionLabel(a) || '').localeCompare(getConnectionLabel(b) || '')
    )
  })
}

export function buildLoadingState(connections: ProviderConnectionLike[]): Record<string, boolean> {
  const next: Record<string, boolean> = {}
  connections.forEach((c) => {
    next[c.id] = true
  })
  return next
}

export function filterQuotaStateByConnections<V>(
  state: Record<string, V>,
  connections: ProviderConnectionLike[],
): Record<string, V> {
  const visibleIds = new Set(connections.map((c) => c.id))
  return Object.fromEntries(Object.entries(state).filter(([id]) => visibleIds.has(id)))
}

export function getConnectionsPageRange(pagination: Pagination): { start: number; end: number } {
  if (!pagination.total) return { start: 0, end: 0 }
  const start = (pagination.page - 1) * pagination.pageSize + 1
  const end = Math.min(pagination.page * pagination.pageSize, pagination.total)
  return { start, end }
}

export function getConnectionsEmptyMessage(
  totals: Totals,
  providerFilter: string,
  accountFilter: string,
): { icon: string; title: string; description: string } {
  if (!totals.eligibleConnections) {
    return {
      icon: 'cloud_off',
      title: 'No Providers Connected',
      description: 'Connect to providers with OAuth to track your API quota limits and usage.',
    }
  }
  if (!totals.providerFilteredConnections) {
    return {
      icon: 'filter_alt_off',
      title: 'No Accounts Match Current Filters',
      description:
        providerFilter === 'all'
          ? 'Try changing the account status filter to see more quota trackers.'
          : `No ${accountFilter === 'inactive' ? 'turned off' : accountFilter === 'active' ? 'active' : 'matching'} accounts found for ${providerFilter}.`,
    }
  }
  return {
    icon: 'filter_alt_off',
    title: 'No Accounts On This Page',
    description: 'Try moving to another page or refreshing the current filters.',
  }
}

export function getPageSizeLabel(pageSize: number, isCustomPageSize: boolean): string {
  return isCustomPageSize ? `Custom: ${pageSize} / page` : `${pageSize} / page`
}

export function getConnectionsPaginationSummary(pagination: Pagination): string {
  const { start, end } = getConnectionsPageRange(pagination)
  return `Showing ${start}-${end} of ${pagination.total}`
}

export function shouldResetPage(previousValue: string, nextValue: string): boolean {
  return previousValue !== nextValue
}

export function getProviderOptions(dataProviderOptions?: string[]): string[] {
  return dataProviderOptions || []
}

export function getSafePagination(pagination: Pagination | undefined, fallbackPageSize: number): Pagination {
  return (
    pagination || {
      page: 1,
      pageSize: fallbackPageSize,
      total: 0,
      totalPages: 1,
    }
  )
}

export function getSafeTotals(
  totals: Totals | undefined,
  fallbackTotal = 0,
  paginationTotal?: number,
): Totals {
  return (
    totals || {
      eligibleConnections: fallbackTotal,
      providerFilteredConnections: paginationTotal ?? fallbackTotal,
    }
  )
}

// ─── Quota cache (localStorage) ─────────────────────────────────────────────
export function getQuotaCache(): Record<string, QuotaEntry> {
  if (typeof window === 'undefined') return {}
  try {
    const cached = window.localStorage.getItem(QUOTA_CACHE_KEY)
    return cached ? JSON.parse(cached) : {}
  } catch (error) {
    console.error('Error reading quota cache:', error)
    return {}
  }
}

export function setQuotaCache(connectionId: string, quotaEntry: QuotaEntry): void {
  if (typeof window === 'undefined') return
  try {
    const cache = getQuotaCache()
    cache[connectionId] = {
      ...quotaEntry,
      cachedAt: new Date().toISOString(),
    }
    window.localStorage.setItem(QUOTA_CACHE_KEY, JSON.stringify(cache))
  } catch (error) {
    console.error('Error writing quota cache:', error)
  }
}

// ─── Formatting ──────────────────────────────────────────────────────────────
export function formatResetTime(date?: string | Date | null): string {
  if (!date) return '-'
  try {
    const resetDate = typeof date === 'string' ? new Date(date) : date
    const now = new Date()
    const diffMs = resetDate.getTime() - now.getTime()
    if (diffMs <= 0) return '-'

    const totalMinutes = Math.ceil(diffMs / (1000 * 60))
    if (totalMinutes < 60) return `${totalMinutes}m`

    const totalHours = Math.floor(totalMinutes / 60)
    const remainingMinutes = totalMinutes % 60
    if (totalHours < 24) return `${totalHours}h ${remainingMinutes}m`

    const days = Math.floor(totalHours / 24)
    const remainingHours = totalHours % 24
    return `${days}d ${remainingHours}h ${remainingMinutes}m`
  } catch {
    return '-'
  }
}

// Today, 12:00 PM style absolute display for a reset timestamp
export function formatResetTimeDisplay(resetTime?: string | Date | null): string | null {
  if (!resetTime) return null
  try {
    const date = new Date(resetTime)
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const tomorrow = new Date(today)
    tomorrow.setDate(tomorrow.getDate() + 1)

    let dayStr: string
    if (date >= today && date < tomorrow) {
      dayStr = 'Today'
    } else if (date >= tomorrow && date < new Date(tomorrow.getTime() + 24 * 60 * 60 * 1000)) {
      dayStr = 'Tomorrow'
    } else {
      dayStr = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    }

    const timeStr = date.toLocaleTimeString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    })
    return `${dayStr}, ${timeStr}`
  } catch {
    return null
  }
}

export function getStatusColor(percentage: number): 'green' | 'yellow' | 'red' {
  if (percentage > 70) return 'green'
  if (percentage >= 30) return 'yellow'
  return 'red'
}

export function getStatusEmoji(percentage: number): string {
  if (percentage > 70) return '🟢'
  if (percentage >= 30) return '🟡'
  return '🔴'
}

export function calculatePercentage(used?: number, total?: number): number {
  if (!total || total === 0) return 0
  if (!used || used < 0) return 100
  if (used >= total) return 0
  return Math.round(((total - used) / total) * 100)
}

export function getRemainingPercentage(quota: Partial<NormalizedQuota>): number {
  if (quota?.remaining !== undefined) return Math.max(0, Math.round(quota.remaining))
  if (quota?.remainingPercentage !== undefined) return Math.round(quota.remainingPercentage)
  return calculatePercentage(quota?.used, quota?.total)
}

// ─── Quota visibility (hide/show rows, persisted via /api/settings) ─────────
export function getQuotaVisibilityKey(quota: Partial<NormalizedQuota> | null | undefined): string {
  if (!quota || typeof quota !== 'object') return ''
  return String(quota.modelKey || quota.name || '').trim()
}

export interface QuotaVisibility {
  [provider: string]: { hidden?: string[] } | undefined
}

function trimHiddenQuotaKeys(hidden: string[], quotas: NormalizedQuota[]): string[] {
  const validKeys = new Set(quotas.map(getQuotaVisibilityKey).filter(Boolean))
  return [...new Set(hidden.map((k) => String(k).trim()).filter((k) => validKeys.has(k)))]
}

function getProviderHiddenQuotaSet(
  provider: string,
  quotaVisibility: QuotaVisibility,
  quotas: NormalizedQuota[],
): Set<string> {
  const hidden = quotaVisibility?.[provider]?.hidden
  if (!Array.isArray(hidden) || hidden.length === 0) return new Set()
  const trimmed = quotas.length > 0 ? trimHiddenQuotaKeys(hidden, quotas) : hidden
  return new Set(trimmed.map(String))
}

export function filterQuotasByVisibility(
  provider: string,
  quotas: NormalizedQuota[],
  quotaVisibility: QuotaVisibility,
): NormalizedQuota[] {
  if (!Array.isArray(quotas) || quotas.length === 0) return []
  const hidden = getProviderHiddenQuotaSet(provider, quotaVisibility, quotas)
  if (hidden.size === 0) return quotas
  return quotas.filter((quota) => !hidden.has(getQuotaVisibilityKey(quota)))
}

export function getHiddenQuotaRows(
  provider: string,
  quotas: NormalizedQuota[],
  quotaVisibility: QuotaVisibility,
): NormalizedQuota[] {
  if (!Array.isArray(quotas) || quotas.length === 0) return []
  const hidden = getProviderHiddenQuotaSet(provider, quotaVisibility, quotas)
  if (hidden.size === 0) return []
  return quotas.filter((quota) => hidden.has(getQuotaVisibilityKey(quota)))
}

// ─── Provider-specific quota parsing (port of parseQuotaData) ────────────────
// The Go backend /api/usage/{id} returns a normalized
// { plan, quotas: { name: { used, total, remainingPercentage, resetAt, ... } } }
// map for all providers, so the per-provider branches of the Next.js parser
// collapse into one generic mapping. Key order is preserved by Object.entries.
export function parseQuotaData(provider: string, data: unknown): NormalizedQuota[] {
  if (!data || typeof data !== 'object') return []
  const d = data as { quotas?: Record<string, Record<string, unknown>>; message?: string }
  const normalized: NormalizedQuota[] = []

  if (d.message && !d.quotas) {
    normalized.push({
      name: 'error',
      used: 0,
      total: 0,
      resetAt: null,
      message: d.message,
    })
    return normalized
  }

  if (!d.quotas) return []

  try {
    Object.entries(d.quotas).forEach(([name, quota]) => {
      const q = quota || {}
      const used = Number(q.used) || 0
      const total = Number(q.total) || 0
      const row: NormalizedQuota = {
        name,
        used,
        total,
        resetAt: (q.resetAt as string) || null,
      }
      if (q.remaining !== undefined) row.remaining = Number(q.remaining)
      if (q.remainingPercentage !== undefined) {
        row.remainingPercentage = Number(q.remainingPercentage)
      } else if (q.remaining === undefined) {
        // Match Next.js claude branch: derive from used/total when neither is present
        row.remainingPercentage = calculatePercentage(used, total)
      }
      if (q.unlimited !== undefined) row.unlimited = Boolean(q.unlimited)
      if (q.recurring !== undefined) row.recurring = q.recurring !== false
      if (q.isCreditBalance !== undefined) row.isCreditBalance = Boolean(q.isCreditBalance)
      if (q.currency !== undefined) row.currency = String(q.currency)
      if (q.displayName !== undefined) row.displayName = String(q.displayName)
      if (q.quotaType !== undefined) row.quotaType = String(q.quotaType)
      normalized.push(row)
    })
  } catch (error) {
    console.error(`Error parsing quota data for ${provider}:`, error)
    return []
  }

  if (provider?.toLowerCase() === 'claude') {
    const CLAUDE_QUOTA_ORDER: Record<string, number> = {
      'session (5h)': 0,
      'weekly (7d)': 1,
      'weekly fable (7d)': 2,
      'weekly opus (7d)': 3,
      'weekly sonnet (7d)': 4,
    }
    normalized.sort(
      (a, b) => (CLAUDE_QUOTA_ORDER[a.name] ?? 99) - (CLAUDE_QUOTA_ORDER[b.name] ?? 99),
    )
  }

  return normalized
}
