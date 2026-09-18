<script lang="ts">
  import { onMount } from 'svelte'
  import {
    AlertCircle,
    Check,
    ChevronDown,
    Copy,
    ExternalLink,
    FlaskConical,
    Globe,
    Loader2,
    Network,
    Pencil,
    Plus,
    RefreshCw,
    Rocket,
    Shield,
    Trash2,
    Upload,
    X
  } from 'lucide-svelte'
  import Card from '../lib/ui/Card.svelte'
  import Toggle from '../lib/ui/Toggle.svelte'
  import { api, type ProxyPool } from '../api/client'

  let proxyPools = $state<ProxyPool[]>([])
  let isLoading = $state(true)
  let isDeployDropdownOpen = $state(false)
  let testingPoolIds = $state<Set<string>>(new Set())
  let testResults = $state<Record<string, { status: string; latency?: number; error?: string }>>({})

  // Modals
  let showAddEditModal = $state(false)
  let editingPool = $state<ProxyPool | null>(null)
  let poolForm = $state({
    name: '',
    type: 'http',
    proxyUrl: '',
    noProxy: '',
    strictProxy: false,
    isActive: true,
  })
  let isSavingPool = $state(false)

  // Batch import modal
  let showBatchImportModal = $state(false)
  let batchImportText = $state('')
  let isImportingBatch = $state(false)

  // Deploy modals
  let showVercelModal = $state(false)
  let vercelForm = $state({ token: '', projectName: '' })
  let isDeployingVercel = $state(false)

  let showCloudflareModal = $state(false)
  let cloudflareForm = $state({ accountId: '', apiToken: '', workerName: '' })
  let isDeployingCloudflare = $state(false)

  let showDenoModal = $state(false)
  let denoForm = $state({ token: '', orgDomain: '', projectName: '' })
  let isDeployingDeno = $state(false)

  // Delete modal
  let deletingPool = $state<ProxyPool | null>(null)

  async function loadData() {
    try {
      isLoading = true
      proxyPools = await api.getProxyPools(true)
    } finally {
      isLoading = false
    }
  }

  onMount(() => {
    loadData()
  })

  function openAddModal() {
    editingPool = null
    poolForm = {
      name: '',
      type: 'http',
      proxyUrl: '',
      noProxy: '',
      strictProxy: false,
      isActive: true,
    }
    showAddEditModal = true
  }

  function openEditModal(pool: ProxyPool) {
    editingPool = pool
    poolForm = {
      name: pool.name || '',
      type: pool.type || 'http',
      proxyUrl: pool.proxyUrl || (pool.urls && pool.urls[0]) || '',
      noProxy: pool.noProxy || '',
      strictProxy: !!pool.strictProxy,
      isActive: pool.isActive !== false,
    }
    showAddEditModal = true
  }

  async function handleSavePool(e: SubmitEvent) {
    e.preventDefault()
    if (!poolForm.name.trim() || !poolForm.proxyUrl.trim()) return

    isSavingPool = true
    try {
      if (editingPool) {
        await api.updateProxyPool(editingPool.id, poolForm)
      } else {
        await api.createProxyPool(poolForm)
      }
      showAddEditModal = false
      await loadData()
    } catch (err) {
      alert(`Failed to save proxy pool: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isSavingPool = false
    }
  }

  async function handleToggleActive(pool: ProxyPool, nextActive: boolean) {
    try {
      await api.updateProxyPool(pool.id, { isActive: nextActive })
      proxyPools = proxyPools.map((p) => (p.id === pool.id ? { ...p, isActive: nextActive } : p))
    } catch (err) {
      alert(`Failed to update status: ${err instanceof Error ? err.message : String(err)}`)
    }
  }

  async function handleTestLatency(pool: ProxyPool) {
    const nextSet = new Set(testingPoolIds)
    nextSet.add(pool.id)
    testingPoolIds = nextSet

    try {
      const res = await api.testProxyPool(pool.id)
      testResults = {
        ...testResults,
        [pool.id]: {
          status: res.status || (res.success ? 'passed' : 'failed'),
          latency: res.latency,
          error: res.error,
        },
      }
    } catch (err) {
      testResults = {
        ...testResults,
        [pool.id]: {
          status: 'failed',
          error: err instanceof Error ? err.message : String(err),
        },
      }
    } finally {
      const s = new Set(testingPoolIds)
      s.delete(pool.id)
      testingPoolIds = s
    }
  }

  async function handleDeletePool() {
    if (!deletingPool) return
    try {
      await api.deleteProxyPool(deletingPool.id)
      proxyPools = proxyPools.filter((p) => p.id !== deletingPool?.id)
      deletingPool = null
    } catch (err) {
      alert(`Failed to delete proxy pool: ${err instanceof Error ? err.message : String(err)}`)
    }
  }

  async function handleBatchImport(e: SubmitEvent) {
    e.preventDefault()
    const lines = batchImportText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)

    if (lines.length === 0) return

    isImportingBatch = true
    try {
      for (const line of lines) {
        // Format: name|url|noProxy or just url
        const parts = line.split('|')
        let name = ''
        let proxyUrl = ''
        let noProxy = ''

        if (parts.length >= 2) {
          name = parts[0].trim()
          proxyUrl = parts[1].trim()
          noProxy = parts[2] ? parts[2].trim() : ''
        } else {
          proxyUrl = parts[0].trim()
          name = `Proxy ${proxyUrl.replace(/^https?:\/\//, '').slice(0, 18)}`
        }

        const type = proxyUrl.startsWith('socks') ? 'socks5' : 'http'
        await api.createProxyPool({
          name,
          proxyUrl,
          noProxy,
          type,
          isActive: true,
        })
      }
      showBatchImportModal = false
      batchImportText = ''
      await loadData()
    } catch (err) {
      alert(`Batch import encountered an error: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isImportingBatch = false
    }
  }

  async function handleDeployVercel(e: SubmitEvent) {
    e.preventDefault()
    if (!vercelForm.token.trim()) return

    isDeployingVercel = true
    try {
      const res = await api.deployVercelRelay({
        vercelToken: vercelForm.token.trim(),
        projectName: vercelForm.projectName.trim() || undefined,
      })
      if (res.error) throw new Error(res.error)
      showVercelModal = false
      vercelForm = { token: '', projectName: '' }
      await loadData()
    } catch (err) {
      alert(`Failed to deploy Vercel relay: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isDeployingVercel = false
    }
  }

  async function handleDeployCloudflare(e: SubmitEvent) {
    e.preventDefault()
    if (!cloudflareForm.accountId.trim() || !cloudflareForm.apiToken.trim()) return

    isDeployingCloudflare = true
    try {
      const res = await api.deployCloudflareRelay({
        accountId: cloudflareForm.accountId.trim(),
        apiToken: cloudflareForm.apiToken.trim(),
        workerName: cloudflareForm.workerName.trim() || undefined,
      })
      if (res.error) throw new Error(res.error)
      showCloudflareModal = false
      cloudflareForm = { accountId: '', apiToken: '', workerName: '' }
      await loadData()
    } catch (err) {
      alert(`Failed to deploy Cloudflare relay: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isDeployingCloudflare = false
    }
  }

  async function handleDeployDeno(e: SubmitEvent) {
    e.preventDefault()
    if (!denoForm.token.trim() || !denoForm.orgDomain.trim()) return

    isDeployingDeno = true
    try {
      const res = await api.deployDenoRelay({
        denoToken: denoForm.token.trim(),
        orgDomain: denoForm.orgDomain.trim(),
        projectName: denoForm.projectName.trim() || undefined,
      })
      if (res.error) throw new Error(res.error)
      showDenoModal = false
      denoForm = { token: '', orgDomain: '', projectName: '' }
      await loadData()
    } catch (err) {
      alert(`Failed to deploy Deno relay: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isDeployingDeno = false
    }
  }

  function getTypeBadgeClass(type: string): string {
    switch (type?.toLowerCase()) {
      case 'vercel':
        return 'bg-zinc-800 text-white border-zinc-700'
      case 'cloudflare':
        return 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20'
      case 'deno':
        return 'bg-teal-500/10 text-teal-600 dark:text-teal-400 border-teal-500/20'
      case 'socks5':
        return 'bg-purple-500/10 text-purple-600 dark:text-purple-400 border-purple-500/20'
      default:
        return 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20'
    }
  }
</script>

<div class="flex flex-col gap-6">
  <!-- PAGE HEADER & ACTION BAR -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
    <div class="space-y-1">
      <div class="flex items-center gap-2">
        <div class="p-2 rounded-lg bg-brand-500/10 text-brand-500">
          <Network class="w-5 h-5" />
        </div>
        <div>
          <h1 class="font-headline text-2xl sm:text-3xl font-bold text-text-main tracking-tight flex items-center gap-2">
            Proxy Pools
          </h1>
          <p class="font-body text-xs sm:text-sm text-text-muted">
            Manage your proxy pool configurations and serverless edge relays
          </p>
        </div>
      </div>
    </div>

    <!-- ACTION BUTTONS -->
    <div class="flex items-center gap-2 flex-wrap relative">
      <!-- Deploy Relay Dropdown -->
      <div class="relative">
        <button
          type="button"
          onclick={() => (isDeployDropdownOpen = !isDeployDropdownOpen)}
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs border border-border transition cursor-pointer shadow-sm"
        >
          <Rocket class="w-4 h-4 text-brand-500" />
          <span>Deploy Relay</span>
          <ChevronDown class="w-3.5 h-3.5 text-text-muted" />
        </button>

        {#if isDeployDropdownOpen}
          <div
            class="absolute right-0 mt-1 w-52 rounded-xl bg-surface border border-border shadow-2xl p-1.5 z-40 space-y-1"
          >
            <button
              type="button"
              onclick={() => {
                isDeployDropdownOpen = false
                showVercelModal = true
              }}
              class="w-full flex items-center gap-2 px-3 py-2 text-xs font-semibold text-text-main rounded-lg hover:bg-surface-2 cursor-pointer transition text-left"
            >
              <span class="w-2 h-2 rounded-full bg-white"></span>
              <span>Deploy Vercel Relay</span>
            </button>
            <button
              type="button"
              onclick={() => {
                isDeployDropdownOpen = false
                showCloudflareModal = true
              }}
              class="w-full flex items-center gap-2 px-3 py-2 text-xs font-semibold text-text-main rounded-lg hover:bg-surface-2 cursor-pointer transition text-left"
            >
              <span class="w-2 h-2 rounded-full bg-orange-500"></span>
              <span>Deploy Cloudflare Relay</span>
            </button>
            <button
              type="button"
              onclick={() => {
                isDeployDropdownOpen = false
                showDenoModal = true
              }}
              class="w-full flex items-center gap-2 px-3 py-2 text-xs font-semibold text-text-main rounded-lg hover:bg-surface-2 cursor-pointer transition text-left"
            >
              <span class="w-2 h-2 rounded-full bg-teal-400"></span>
              <span>Deploy Deno Relay</span>
            </button>
          </div>
        {/if}
      </div>

      <!-- Batch Import -->
      <button
        type="button"
        onclick={() => (showBatchImportModal = true)}
        class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs border border-border transition cursor-pointer shadow-sm"
      >
        <Upload class="w-4 h-4 text-text-muted" />
        <span>Batch Import</span>
      </button>

      <!-- Add Proxy Pool -->
      <button
        type="button"
        onclick={openAddModal}
        class="flex items-center gap-1.5 px-3.5 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs shadow-md shadow-brand-500/20 transition cursor-pointer"
      >
        <Plus class="w-4 h-4" />
        <span>Add Proxy Pool</span>
      </button>
    </div>
  </div>

  <!-- POOLS LIST / TABLE CARD -->
  <Card padding="md" class="space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-border">
      <div class="flex items-center gap-2">
        <Globe class="w-4 h-4 text-brand-500" />
        <h2 class="text-sm font-bold text-text-main">Configured Proxy Pools</h2>
      </div>
      <span class="text-xs text-text-muted font-mono">{proxyPools.length} pools</span>
    </div>

    {#if isLoading}
      <div class="flex flex-col items-center justify-center py-16 gap-3 text-text-muted">
        <Loader2 class="w-7 h-7 animate-spin text-brand-500" />
        <span class="text-xs font-mono">Loading proxy pools...</span>
      </div>
    {:else if proxyPools.length === 0}
      <div class="text-center py-14 space-y-3">
        <div class="inline-flex items-center justify-center w-14 h-14 rounded-full bg-brand-500/10 text-brand-500">
          <Network class="w-7 h-7" />
        </div>
        <div class="space-y-1">
          <p class="text-text-main font-semibold text-sm">No proxy pools configured</p>
          <p class="text-xs text-text-muted max-w-sm mx-auto">
            Deploy a serverless edge relay (Vercel, Cloudflare, Deno) or connect existing HTTP / SOCKS5 proxies to bypass regional rate-limits.
          </p>
        </div>
        <button
          type="button"
          onclick={openAddModal}
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer shadow-sm"
        >
          <Plus class="w-4 h-4" />
          <span>Add Proxy Pool</span>
        </button>
      </div>
    {:else}
      <div class="flex flex-col divide-y divide-border/40">
        {#each proxyPools as pool (pool.id)}
          {@const isTesting = testingPoolIds.has(pool.id)}
          {@const result = testResults[pool.id]}
          <div class="group flex flex-col sm:flex-row sm:items-center justify-between py-3.5 gap-3 transition">
            <!-- Left Info -->
            <div class="space-y-1 min-w-0 flex-1">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-bold text-sm text-text-main">{pool.name || 'Proxy Pool'}</span>

                <!-- Type Badge -->
                <span class="text-[10px] font-mono font-bold uppercase px-2 py-0.5 rounded border {getTypeBadgeClass(pool.type)}">
                  {pool.type || 'http'}
                </span>

                <!-- Bound Connection Count Badge -->
                {#if typeof pool.boundConnectionCount === 'number'}
                  <span class="text-[10px] font-medium px-2 py-0.5 rounded bg-surface-2 border border-border text-text-muted">
                    {pool.boundConnectionCount} {pool.boundConnectionCount === 1 ? 'connection' : 'connections'} bound
                  </span>
                {/if}
              </div>

              <!-- URL Monospace -->
              <div class="flex items-center gap-2">
                <code class="text-xs text-text-muted font-mono truncate max-w-md">
                  {pool.proxyUrl || (pool.urls && pool.urls.join(', ')) || '—'}
                </code>
                {#if pool.noProxy}
                  <span class="text-[11px] text-text-subtle font-mono truncate">
                    (bypass: {pool.noProxy})
                  </span>
                {/if}
              </div>
            </div>

            <!-- Right Controls -->
            <div class="flex items-center gap-3 shrink-0">
              <!-- Latency test status / button -->
              <div class="flex items-center gap-1.5">
                {#if result}
                  <span
                    class="text-xs font-mono font-semibold px-2 py-1 rounded border {result.status === 'passed'
                      ? 'bg-success/10 text-success border-success/20'
                      : 'bg-danger/10 text-danger border-danger/20'}"
                  >
                    {#if result.status === 'passed'}
                      {result.latency ? `${result.latency}ms` : 'Passed'}
                    {:else}
                      Failed
                    {/if}
                  </span>
                {/if}

                <button
                  type="button"
                  onclick={() => handleTestLatency(pool)}
                  disabled={isTesting}
                  class="flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border text-xs font-semibold text-text-main transition cursor-pointer disabled:opacity-50"
                  title="Test proxy latency"
                >
                  {#if isTesting}
                    <Loader2 class="w-3.5 h-3.5 animate-spin text-brand-500" />
                    <span>Testing...</span>
                  {:else}
                    <FlaskConical class="w-3.5 h-3.5 text-text-muted" />
                    <span>Test</span>
                  {/if}
                </button>
              </div>

              <!-- Active Toggle -->
              <Toggle
                checked={pool.isActive}
                size="sm"
                label={pool.isActive ? 'Disable pool' : 'Enable pool'}
                onChange={(next) => handleToggleActive(pool, next)}
              />

              <!-- Edit Button -->
              <button
                type="button"
                onclick={() => openEditModal(pool)}
                class="p-2 hover:bg-surface-2 rounded-lg text-text-muted hover:text-text-main transition cursor-pointer"
                title="Edit pool"
              >
                <Pencil class="w-4 h-4" />
              </button>

              <!-- Delete Button -->
              <button
                type="button"
                onclick={() => (deletingPool = pool)}
                class="p-2 hover:bg-danger/10 rounded-lg text-text-subtle hover:text-danger transition cursor-pointer"
                title="Delete pool"
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </Card>
</div>

<!-- MODAL: Add / Edit Proxy Pool -->
{#if showAddEditModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">
          {editingPool ? 'Edit Proxy Pool' : 'Add Proxy Pool'}
        </h3>
        <button
          type="button"
          onclick={() => (showAddEditModal = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleSavePool} class="space-y-3.5 text-xs">
        <div class="space-y-1">
          <label for="pool-name" class="block font-semibold text-text-muted">Pool Name</label>
          <input
            id="pool-name"
            type="text"
            bind:value={poolForm.name}
            placeholder="e.g. US Residential Relay"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="pool-type" class="block font-semibold text-text-muted">Type</label>
          <select
            id="pool-type"
            bind:value={poolForm.type}
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
          >
            <option value="http">HTTP / HTTPS Proxy</option>
            <option value="socks5">SOCKS5 Proxy</option>
            <option value="vercel">Vercel Edge Relay</option>
            <option value="cloudflare">Cloudflare Worker Relay</option>
            <option value="deno">Deno Deploy Relay</option>
          </select>
        </div>

        <div class="space-y-1">
          <label for="pool-url" class="block font-semibold text-text-muted">Proxy URL</label>
          <input
            id="pool-url"
            type="text"
            bind:value={poolForm.proxyUrl}
            placeholder="http://127.0.0.1:7890 or https://relay.vercel.app"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="pool-noproxy" class="block font-semibold text-text-muted">No Proxy (Optional)</label>
          <input
            id="pool-noproxy"
            type="text"
            bind:value={poolForm.noProxy}
            placeholder="localhost, 127.0.0.1, *.internal"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="flex items-center justify-between pt-2">
          <div>
            <p class="font-semibold text-text-main">Strict Proxy</p>
            <p class="text-[11px] text-text-subtle">Fail request immediately if proxy is unreachable</p>
          </div>
          <Toggle
            checked={poolForm.strictProxy}
            size="sm"
            onChange={(val) => (poolForm.strictProxy = val)}
          />
        </div>

        <div class="flex gap-2 pt-3 border-t border-border">
          <button
            type="submit"
            disabled={isSavingPool}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5"
          >
            {#if isSavingPool}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <span>{editingPool ? 'Update Pool' : 'Create Pool'}</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => (showAddEditModal = false)}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- MODAL: Batch Import -->
{#if showBatchImportModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-lg p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">Batch Import Proxies</h3>
        <button
          type="button"
          onclick={() => (showBatchImportModal = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleBatchImport} class="space-y-3 text-xs">
        <p class="text-text-muted">
          Paste proxy list below (one per line). Format: <code class="font-mono text-brand-500">http://host:port</code> or <code class="font-mono text-brand-500">Name|http://host:port|noProxy</code>.
        </p>

        <textarea
          bind:value={batchImportText}
          rows={6}
          placeholder={`US Proxy 1|http://user:pass@192.168.1.1:8080|localhost\nhttp://127.0.0.1:7890\nsocks5://127.0.0.1:1080`}
          class="w-full p-3 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
          required
        ></textarea>

        <div class="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={isImportingBatch || !batchImportText.trim()}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5 disabled:opacity-50"
          >
            {#if isImportingBatch}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Importing...</span>
            {:else}
              <span>Import Proxies</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => (showBatchImportModal = false)}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- MODAL: Deploy Vercel Relay -->
{#if showVercelModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">Deploy Vercel Relay</h3>
        <button
          type="button"
          onclick={() => (showVercelModal = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleDeployVercel} class="space-y-3.5 text-xs">
        <div class="space-y-1">
          <label for="vercel-token" class="block font-semibold text-text-muted">Vercel API Token</label>
          <input
            id="vercel-token"
            type="password"
            bind:value={vercelForm.token}
            placeholder="your-vercel-api-token"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="vercel-project" class="block font-semibold text-text-muted">Project Name (Optional)</label>
          <input
            id="vercel-project"
            type="text"
            bind:value={vercelForm.projectName}
            placeholder="9router-relay"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={isDeployingVercel || !vercelForm.token.trim()}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5 disabled:opacity-50"
          >
            {#if isDeployingVercel}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Deploying...</span>
            {:else}
              <span>Deploy Relay</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => (showVercelModal = false)}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- MODAL: Deploy Cloudflare Relay -->
{#if showCloudflareModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">Deploy Cloudflare Worker Relay</h3>
        <button
          type="button"
          onclick={() => (showCloudflareModal = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleDeployCloudflare} class="space-y-3.5 text-xs">
        <div class="space-y-1">
          <label for="cf-account-id" class="block font-semibold text-text-muted">Cloudflare Account ID</label>
          <input
            id="cf-account-id"
            type="text"
            bind:value={cloudflareForm.accountId}
            placeholder="your-cloudflare-account-id"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="cf-api-token" class="block font-semibold text-text-muted">Cloudflare API Token</label>
          <input
            id="cf-api-token"
            type="password"
            bind:value={cloudflareForm.apiToken}
            placeholder="Workers Scripts Edit token"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="cf-worker-name" class="block font-semibold text-text-muted">Worker Name (Optional)</label>
          <input
            id="cf-worker-name"
            type="text"
            bind:value={cloudflareForm.workerName}
            placeholder="9router-relay"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={isDeployingCloudflare || !cloudflareForm.accountId.trim() || !cloudflareForm.apiToken.trim()}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5 disabled:opacity-50"
          >
            {#if isDeployingCloudflare}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Deploying...</span>
            {:else}
              <span>Deploy Worker</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => (showCloudflareModal = false)}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- MODAL: Deploy Deno Relay -->
{#if showDenoModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">Deploy Deno Deploy Relay</h3>
        <button
          type="button"
          onclick={() => (showDenoModal = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleDeployDeno} class="space-y-3.5 text-xs">
        <div class="space-y-1">
          <label for="deno-token" class="block font-semibold text-text-muted">Deno Deploy Token</label>
          <input
            id="deno-token"
            type="password"
            bind:value={denoForm.token}
            placeholder="your-deno-deploy-token"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="deno-org" class="block font-semibold text-text-muted">Organization Domain / Slug</label>
          <input
            id="deno-org"
            type="text"
            bind:value={denoForm.orgDomain}
            placeholder="e.g. my-org"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div class="space-y-1">
          <label for="deno-project" class="block font-semibold text-text-muted">Project Name (Optional)</label>
          <input
            id="deno-project"
            type="text"
            bind:value={denoForm.projectName}
            placeholder="relay-9router"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={isDeployingDeno || !denoForm.token.trim() || !denoForm.orgDomain.trim()}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5 disabled:opacity-50"
          >
            {#if isDeployingDeno}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Deploying...</span>
            {:else}
              <span>Deploy Deno</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => (showDenoModal = false)}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- MODAL: Delete Confirmation -->
{#if deletingPool}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">Delete Proxy Pool</h3>
        <button
          type="button"
          onclick={() => (deletingPool = null)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <p class="text-sm text-text-muted leading-relaxed">
        Are you sure you want to delete proxy pool <span class="font-bold text-text-main">"{deletingPool.name}"</span>?
        Connections assigned to this proxy will revert to direct network transport.
      </p>

      <div class="flex gap-2 pt-2">
        <button
          type="button"
          onclick={handleDeletePool}
          class="flex-1 py-2 px-4 rounded-lg bg-danger hover:bg-danger/90 text-white font-semibold text-xs transition cursor-pointer"
        >
          Delete Pool
        </button>
        <button
          type="button"
          onclick={() => (deletingPool = null)}
          class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
        >
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}
