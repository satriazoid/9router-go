<script lang="ts">
  import { onMount } from 'svelte'
  import {
    api,
    type ConnectionQuotaInfo,
    type ConnectionUsageResponse,
    type ProviderConnection
  } from '../api/client'
  import Toggle from '../lib/ui/Toggle.svelte'
  import { getIconPath } from './connections/types'

  interface Props {
    connections?: ProviderConnection[]
  }

  let { connections: initialConns = [] }: Props = $props()
  let connections = $state<ProviderConnection[]>([])
  let isLoading = $state(true)
  let isRefreshing = $state(false)
  let quotaData = $state<Record<string, ConnectionUsageResponse>>({})
  let quotaLoading = $state<Record<string, boolean>>({})
  let quotaErrors = $state<Record<string, string>>({})

  // Filters
  let selectedProvider = $state('all')
  let accountFilter = $state<'all' | 'active' | 'inactive'>('all')
  let isProviderDropdownOpen = $state(false)
  let expiringFirst = $state(false)

  // Auto-refresh
  let autoRefresh = $state(true)
  let countdown = $state(60)

  $effect(() => {
    if (initialConns.length > 0 && connections.length === 0) {
      connections = initialConns
      isLoading = false
      fetchAllQuotas(initialConns)
    }
  })

  // Load all connections
  async function loadConnections() {
    try {
      const res = await api.getProvidersClient()
      connections = res.connections || []
      // Fetch quotas for each connection
      fetchAllQuotas(connections)
    } catch {
      const fallbackConns = await api.getConnections().catch(() => [])
      connections = fallbackConns
      fetchAllQuotas(fallbackConns)
    } finally {
      isLoading = false
      isRefreshing = false
    }
  }

  // Fetch quota for one connection
  async function fetchQuotaForConnection(conn: ProviderConnection, force = false) {
    const id = conn.id
    quotaLoading[id] = true
    try {
      const usage = await api.getConnectionUsage(id, force)
      quotaData[id] = usage
      if (usage.error) {
        quotaErrors[id] = usage.error
      } else {
        delete quotaErrors[id]
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      quotaErrors[id] = msg
    } finally {
      quotaLoading[id] = false
    }
  }

  async function fetchAllQuotas(conns: ProviderConnection[], force = false) {
    await Promise.allSettled(conns.map((c) => fetchQuotaForConnection(c, force)))
  }

  async function handleRefreshAll() {
    isRefreshing = true
    countdown = 60
    await loadConnections()
  }

  // Toggle connection active/inactive
  async function handleToggle(conn: ProviderConnection) {
    const currentActive = conn.isActive === 1 || conn.isActive === true
    const newActive = !currentActive
    try {
      await api.updateConnection(conn.id, { isActive: newActive ? 1 : 0 })
      connections = connections.map((c) =>
        c.id === conn.id ? { ...c, isActive: newActive ? 1 : 0 } : c
      )
    } catch (err) {
      console.error('Failed to update connection status:', err)
    }
  }

  // Turn off empty accounts
  async function handleTurnOffEmpty() {
    const emptyConns = connections.filter((c) => {
      const isActive = c.isActive === 1 || c.isActive === true
      if (!isActive) return false
      const usage = quotaData[c.id]
      if (!usage || !usage.quotas) return false
      const qList = getQuotaList(usage.quotas)
      if (qList.length === 0) return false
      // If every quota has 0% or remaining <= 0
      return qList.every((q) => getRemainingPercentage(q) <= 0)
    })

    await Promise.allSettled(
      emptyConns.map((c) =>
        api
          .updateConnection(c.id, { isActive: 0 })
          .then(() => {
            connections = connections.map((item) =>
              item.id === c.id ? { ...item, isActive: 0 } : item
            )
          })
          .catch(() => {})
      )
    )
  }

  // Turn on available accounts
  async function handleTurnOnAvailable() {
    const availableConns = connections.filter((c) => {
      const isActive = c.isActive === 1 || c.isActive === true
      if (isActive) return false
      const usage = quotaData[c.id]
      if (!usage || !usage.quotas) return false
      const qList = getQuotaList(usage.quotas)
      if (qList.length === 0) return false
      // If at least one quota has remaining > 0
      return qList.some((q) => getRemainingPercentage(q) > 0)
    })

    await Promise.allSettled(
      availableConns.map((c) =>
        api
          .updateConnection(c.id, { isActive: 1 })
          .then(() => {
            connections = connections.map((item) =>
              item.id === c.id ? { ...item, isActive: 1 } : item
            )
          })
          .catch(() => {})
      )
    )
  }

  onMount(() => {
    loadConnections()

    const timer = setInterval(() => {
      if (!autoRefresh) return
      countdown -= 1
      if (countdown <= 0) {
        countdown = 60
        loadConnections()
      }
    }, 1000)

    function handleDocClick(e: MouseEvent) {
      const target = e.target as HTMLElement | null
      if (!target?.closest('#provider-dropdown-container')) {
        isProviderDropdownOpen = false
      }
    }
    document.addEventListener('click', handleDocClick)

    return () => {
      clearInterval(timer)
      document.removeEventListener('click', handleDocClick)
    }
  })

  // Helpers
  function getDisplayName(conn: ProviderConnection): string {
    return (
      conn.name ||
      conn.displayName ||
      conn.email ||
      (conn.authType === 'oauth' ? 'OAuth Account' : 'API Key Slot')
    )
  }

  function getQuotaList(
    quotas?: Record<string, ConnectionQuotaInfo> | ConnectionQuotaInfo[]
  ): ConnectionQuotaInfo[] {
    if (!quotas) return []
    if (Array.isArray(quotas)) return quotas
    return Object.entries(quotas).map(([key, info]) => ({
      ...info,
      modelKey: key,
    }))
  }

  function getRemainingPercentage(quota: ConnectionQuotaInfo): number {
    if (quota.remainingPercentage !== undefined) {
      return Math.max(0, Math.min(100, Math.round(quota.remainingPercentage)))
    }
    if (quota.remaining !== undefined && quota.total && quota.total > 0) {
      return Math.max(0, Math.min(100, Math.round((quota.remaining / quota.total) * 100)))
    }
    if (quota.used !== undefined && quota.total && quota.total > 0) {
      const rem = Math.max(0, quota.total - quota.used)
      return Math.max(0, Math.min(100, Math.round((rem / quota.total) * 100)))
    }
    return 100
  }

  function formatResetTime(resetAt?: string): string | null {
    if (!resetAt) return null
    try {
      const target = new Date(resetAt).getTime()
      const now = Date.now()
      const diff = target - now
      if (diff <= 0) return 'Resetting soon'
      const hours = Math.floor(diff / (1000 * 60 * 60))
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
      if (hours >= 24) {
        const days = Math.floor(hours / 24)
        const remHours = hours % 24
        return `Resets in ${days}d ${remHours}h`
      }
      return `Resets in ${hours}h ${minutes}m`
    } catch {
      return null
    }
  }

  function getEarliestResetTime(conn: ProviderConnection): number {
    const usage = quotaData[conn.id]
    if (!usage || !usage.quotas) return Infinity
    const list = getQuotaList(usage.quotas)
    let earliest = Infinity
    for (const q of list) {
      if (q.resetAt) {
        const t = new Date(q.resetAt).getTime()
        if (t > Date.now() && t < earliest) {
          earliest = t
        }
      }
    }
    return earliest
  }

  // Filtered connections
  let uniqueProviders = $derived(
    Array.from(new Set(connections.map((c) => c.provider))).sort()
  )

  let filteredConnections = $derived(
    connections.filter((conn) => {
      // Provider filter
      if (selectedProvider !== 'all' && conn.provider !== selectedProvider) {
        return false
      }
      // Active / Inactive filter
      const isActive = conn.isActive === 1 || conn.isActive === true
      if (accountFilter === 'active' && !isActive) return false
      if (accountFilter === 'inactive' && isActive) return false
      return true
    })
  )

  let sortedConnections = $derived(
    expiringFirst
      ? [...filteredConnections].sort(
          (a, b) => getEarliestResetTime(a) - getEarliestResetTime(b)
        )
      : filteredConnections
  )

  let activeCount = $derived(
    connections.filter((c) => c.isActive === 1 || c.isActive === true).length
  )
  let inactiveCount = $derived(connections.length - activeCount)
</script>

<div class="space-y-6">
  <!-- Header -->
  <div class="flex items-center justify-between gap-4 flex-wrap">
    <div class="flex items-center gap-3">
      <div
        class="size-10 rounded-xl bg-brand-500/10 text-brand-600 dark:text-brand-400 flex items-center justify-center shrink-0"
      >
        <span class="material-symbols-outlined text-[24px]">data_usage</span>
      </div>
      <div>
        <h1 class="text-xl font-bold text-text-main tracking-tight">Quota Tracker</h1>
        <p class="text-xs text-text-muted">Track and manage your API quota limits</p>
      </div>
    </div>
  </div>

  <!-- Action Bar -->
  <div
    class="flex items-center justify-between gap-3 flex-wrap p-2.5 rounded-xl bg-surface border border-border-subtle shadow-[var(--shadow-soft)]"
  >
    <!-- Left: Provider dropdown + Account segmented tabs -->
    <div class="flex items-center gap-2 flex-wrap">
      <!-- Provider filter dropdown -->
      <div class="relative" id="provider-dropdown-container">
        <button
          type="button"
          onclick={() => (isProviderDropdownOpen = !isProviderDropdownOpen)}
          class="flex h-8 items-center gap-1.5 px-2.5 rounded-lg border border-border-subtle bg-surface-2 text-xs font-medium text-text-main hover:bg-surface-3 transition-colors cursor-pointer"
        >
          <span class="material-symbols-outlined text-[16px] text-text-muted">apps</span>
          <span class="capitalize">
            {selectedProvider === 'all' ? 'All Providers' : selectedProvider}
          </span>
          <span
            class="material-symbols-outlined text-[14px] text-text-muted transition-transform duration-150"
            style:transform={isProviderDropdownOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
          >
            expand_more
          </span>
        </button>

        {#if isProviderDropdownOpen}
          <div
            class="absolute left-0 top-full mt-1.5 w-48 rounded-xl bg-surface border border-border-subtle shadow-xl z-30 py-1 max-h-64 overflow-y-auto custom-scrollbar"
          >
            <button
              type="button"
              onclick={() => {
                selectedProvider = 'all'
                isProviderDropdownOpen = false
              }}
              class="flex items-center justify-between w-full px-3 py-1.5 text-xs text-left hover:bg-surface-2 transition-colors cursor-pointer {selectedProvider ===
              'all'
                ? 'text-brand-500 font-semibold'
                : 'text-text-main'}"
            >
              <span>All Providers</span>
              <span class="text-text-muted text-[11px]">{connections.length}</span>
            </button>
            <div class="h-px bg-border-subtle my-1"></div>
            {#each uniqueProviders as prov}
              {@const count = connections.filter((c) => c.provider === prov).length}
              <button
                type="button"
                onclick={() => {
                  selectedProvider = prov
                  isProviderDropdownOpen = false
                }}
                class="flex items-center justify-between w-full px-3 py-1.5 text-xs text-left capitalize hover:bg-surface-2 transition-colors cursor-pointer {selectedProvider ===
                prov
                  ? 'text-brand-500 font-semibold'
                  : 'text-text-main'}"
              >
                <span>{prov}</span>
                <span class="text-text-muted text-[11px]">{count}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Account Status Tabs -->
      <div class="flex items-center rounded-lg bg-surface-2 p-0.5 border border-border-subtle">
        <button
          type="button"
          onclick={() => (accountFilter = 'all')}
          class="px-2.5 py-1 text-xs rounded-md font-medium transition-colors cursor-pointer {accountFilter ===
          'all'
            ? 'bg-surface text-text-main shadow-xs'
            : 'text-text-muted hover:text-text-main'}"
        >
          All accounts
        </button>
        <button
          type="button"
          onclick={() => (accountFilter = 'active')}
          class="px-2.5 py-1 text-xs rounded-md font-medium transition-colors cursor-pointer {accountFilter ===
          'active'
            ? 'bg-surface text-text-main shadow-xs'
            : 'text-text-muted hover:text-text-main'}"
        >
          Active ({activeCount})
        </button>
        <button
          type="button"
          onclick={() => (accountFilter = 'inactive')}
          class="px-2.5 py-1 text-xs rounded-md font-medium transition-colors cursor-pointer {accountFilter ===
          'inactive'
            ? 'bg-surface text-text-main shadow-xs'
            : 'text-text-muted hover:text-text-main'}"
        >
          Turned off ({inactiveCount})
        </button>
      </div>
    </div>

    <!-- Right: Action pills -->
    <div class="flex items-center gap-1.5 flex-wrap">
      <!-- Expiring first -->
      <button
        type="button"
        onclick={() => (expiringFirst = !expiringFirst)}
        class="flex h-8 items-center gap-1 rounded-lg border px-2.5 text-xs transition-colors cursor-pointer {expiringFirst
          ? 'border-amber-500/40 bg-amber-500/10 text-amber-500 font-medium'
          : 'border-border-subtle bg-surface text-text-muted hover:bg-surface-2 hover:text-text-main'}"
        title="Sort accounts by earliest quota reset time"
      >
        <span class="material-symbols-outlined text-[14px]">hourglass_top</span>
        <span class="hidden sm:inline">Expiring first</span>
      </button>

      <!-- Turn off Empty -->
      <button
        type="button"
        onclick={handleTurnOffEmpty}
        class="flex h-8 items-center gap-1 rounded-lg border border-red-500/30 bg-red-500/5 px-2.5 text-xs text-red-500 transition-colors hover:bg-red-500/10 cursor-pointer"
        title="Disable connections with depleted quota"
      >
        <span class="material-symbols-outlined text-[14px]">block</span>
        <span class="hidden sm:inline">Turn off Empty</span>
      </button>

      <!-- Turn on Available -->
      <button
        type="button"
        onclick={handleTurnOnAvailable}
        class="flex h-8 items-center gap-1 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-2.5 text-xs text-emerald-500 transition-colors hover:bg-emerald-500/10 cursor-pointer"
        title="Enable connections that still have quota"
      >
        <span class="material-symbols-outlined text-[14px]">check_circle</span>
        <span class="hidden sm:inline">Turn on Available</span>
      </button>

      <!-- Auto-refresh -->
      <button
        type="button"
        onclick={() => (autoRefresh = !autoRefresh)}
        class="flex h-8 items-center gap-1 rounded-lg border border-border-subtle bg-surface px-2.5 text-xs transition-colors hover:bg-surface-2 cursor-pointer"
        title={autoRefresh ? 'Disable auto-refresh' : 'Enable auto-refresh'}
      >
        <span
          class="material-symbols-outlined text-[16px] {autoRefresh
            ? 'text-brand-500'
            : 'text-text-muted'}"
        >
          {autoRefresh ? 'toggle_on' : 'toggle_off'}
        </span>
        <span class="hidden text-text-main sm:inline">Auto-refresh</span>
        {#if autoRefresh}
          <span class="text-[10px] text-text-muted tabular-nums">({countdown}s)</span>
        {/if}
      </button>

      <!-- Manual refresh -->
      <button
        type="button"
        onclick={handleRefreshAll}
        disabled={isRefreshing}
        class="flex h-8 items-center justify-center rounded-lg border border-border-subtle bg-surface px-2 text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
        title="Refresh all"
        aria-label="Refresh quota data"
      >
        <span class="material-symbols-outlined text-[16px] {isRefreshing ? 'animate-spin' : ''}">
          refresh
        </span>
      </button>
    </div>
  </div>

  <!-- Account Cards Grid -->
  {#if isLoading}
    <div class="flex items-center justify-center py-20 text-text-muted">
      <span class="material-symbols-outlined animate-spin mr-2 text-2xl">progress_activity</span>
      <span>Loading accounts and quotas...</span>
    </div>
  {:else if sortedConnections.length === 0}
    <div
      class="p-12 text-center rounded-2xl bg-surface border border-border-subtle shadow-[var(--shadow-soft)]"
    >
      <span class="material-symbols-outlined text-4xl text-text-muted mb-2">inbox</span>
      <h3 class="text-base font-semibold text-text-main">No accounts found</h3>
      <p class="text-xs text-text-muted mt-1">
        Try adjusting your provider or status filter to see more connections.
      </p>
    </div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      {#each sortedConnections as conn (conn.id)}
        {@const isActive = conn.isActive === 1 || conn.isActive === true}
        {@const usage = quotaData[conn.id]}
        {@const isQuotaLoading = quotaLoading[conn.id]}
        {@const error = quotaErrors[conn.id] || conn.lastError}
        {@const quotas = getQuotaList(usage?.quotas)}

        <div
          class="flex flex-col p-4 rounded-xl border bg-surface transition-all {isActive
            ? 'border-border-subtle shadow-[var(--shadow-soft)] hover:border-brand-500/30'
            : 'border-border-subtle/60 opacity-70 bg-surface-2/40'}"
        >
          <!-- Card Header -->
          <div class="flex items-center justify-between gap-3 pb-3 border-b border-border-subtle">
            <!-- Provider logo & Name -->
            <div class="flex items-center gap-3 min-w-0">
              <div
                class="size-9 rounded-lg bg-surface-2 border border-border-subtle flex items-center justify-center shrink-0 overflow-hidden p-1"
              >
                <img
                  src={getIconPath(conn.provider)}
                  alt={conn.provider}
                  class="size-full object-contain"
                  onerror={(e) => {
                    const el = e.currentTarget as HTMLImageElement
                    el.style.display = 'none'
                  }}
                />
              </div>

              <div class="min-w-0">
                <div class="flex items-center gap-1.5">
                  <h3 class="font-semibold text-sm text-text-main truncate">
                    {getDisplayName(conn)}
                  </h3>
                </div>
                <div class="flex items-center gap-2 text-[11px] text-text-muted capitalize">
                  <span>{conn.provider}</span>
                  <span>•</span>
                  <span>{conn.authType}</span>
                </div>
              </div>
            </div>

            <!-- Status & Quick Toggle -->
            <div class="flex items-center gap-2 shrink-0">
              <span
                class="px-2 py-0.5 rounded-full text-[10px] font-semibold uppercase tracking-wider {isActive
                  ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                  : 'bg-surface-3 text-text-muted border border-border-subtle'}"
              >
                {isActive ? 'Active' : 'Turned off'}
              </span>

              <Toggle
                checked={isActive}
                onChange={() => handleToggle(conn)}
                title={isActive ? 'Deactivate connection' : 'Activate connection'}
              />
            </div>
          </div>

          <!-- Card Body / Quota Breakdown -->
          <div class="pt-3 flex-1 flex flex-col justify-between gap-3">
            {#if isQuotaLoading}
              <div class="flex items-center gap-2 py-4 text-xs text-text-muted justify-center">
                <span class="material-symbols-outlined text-[16px] animate-spin">
                  progress_activity
                </span>
                <span>Refreshing quota...</span>
              </div>
            {:else if error}
              <div
                class="p-2.5 rounded-lg bg-red-500/10 border border-red-500/20 text-red-500 text-xs flex items-start gap-2"
              >
                <span class="material-symbols-outlined text-[16px] shrink-0 mt-0.5">error</span>
                <span class="truncate leading-relaxed">{error}</span>
              </div>
            {:else if quotas.length > 0}
              <div class="space-y-2.5">
                {#each quotas.slice(0, 4) as quota (quota.modelKey || quota.name)}
                  {@const remainingPct = getRemainingPercentage(quota)}
                  {@const resetText = formatResetTime(quota.resetAt)}

                  <div>
                    <div class="flex items-center justify-between text-xs mb-1">
                      <span class="font-medium text-text-main truncate max-w-[200px]">
                        {quota.displayName || quota.name || quota.modelKey}
                      </span>
                      <div class="flex items-center gap-2">
                        {#if resetText}
                          <span class="text-[10px] text-text-muted">{resetText}</span>
                        {/if}
                        <span class="font-mono text-[11px] text-text-muted font-semibold">
                          {remainingPct}%
                        </span>
                      </div>
                    </div>

                    <!-- Progress bar -->
                    <div class="h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                      <div
                        class="h-full transition-all duration-300 rounded-full {remainingPct > 50
                          ? 'bg-emerald-500'
                          : remainingPct > 20
                            ? 'bg-amber-500'
                            : 'bg-red-500'}"
                        style:width={`${remainingPct}%`}
                      ></div>
                    </div>
                  </div>
                {/each}

                {#if quotas.length > 4}
                  <p class="text-[11px] text-text-muted text-center pt-1">
                    + {quotas.length - 4} more model quotas
                  </p>
                {/if}
              </div>
            {:else}
              <div class="py-3 text-center text-xs text-text-muted">
                <span>Account active. No quota limits tracked.</span>
              </div>
            {/if}

            <!-- Card Footer: Plan and re-check button -->
            <div class="flex items-center justify-between pt-2 text-[11px] text-text-muted border-t border-border-subtle/50">
              <span>{usage?.plan || 'Standard Plan'}</span>
              <button
                type="button"
                onclick={() => fetchQuotaForConnection(conn, true)}
                disabled={isQuotaLoading}
                class="hover:text-text-main inline-flex items-center gap-1 cursor-pointer disabled:opacity-50"
              >
                <span class="material-symbols-outlined text-[13px] {isQuotaLoading ? 'animate-spin' : ''}">
                  refresh
                </span>
                <span>Check</span>
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
