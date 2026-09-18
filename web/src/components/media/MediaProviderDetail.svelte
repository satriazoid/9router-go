<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type APIKey, type ProviderConnection } from '../../api/client'
  import { getModelKind, getModelsByProviderId } from '../../lib/models'
  import type { ProviderCatalogItem } from '../../lib/providers'
  import Card from '../../lib/ui/Card.svelte'
  import { getIconPath } from '../connections/types'
  import MediaModelCard from './MediaModelCard.svelte'
  import { type MediaKind } from './mediaTypes'

  interface Props {
    provider: ProviderCatalogItem
    kind: MediaKind
    connections?: ProviderConnection[]
    apiKeys?: APIKey[]
    onBack: () => void
    onRefresh: () => void
  }

  let { provider, kind, connections = [], apiKeys = [], onBack, onRefresh }: Props = $props()

  let activeApiKey = $state('')
  let selectedConnId = $state('')
  let exampleInput = $state('')
  let searchType = $state('web')
  let maxResults = $state(5)
  let country = $state('')
  let language = $state('')
  let copiedCurl = $state(false)
  let isTestingExample = $state(false)
  let exampleResponse = $state<string | null>(null)
  let exampleLatency = $state<number | null>(null)
  let searchQuery = $state('')
  let copiedModelId = $state<string | null>(null)
  let testingModelId = $state<string | null>(null)
  let modelTestResults = $state<Record<string, { ok: boolean; error?: string; latency?: number }>>({})

  let providerConns = $derived(connections.filter((c) => c.provider === provider.id))
  let icon = $derived(getIconPath(provider.id))
  let rawModels = $derived(getModelsByProviderId(provider.id))
  let mediaModels = $derived(rawModels.filter((m) => getModelKind(m) === kind))
  let filteredModels = $derived(
    mediaModels.filter((m) => {
      if (!searchQuery.trim()) return true
      const q = searchQuery.toLowerCase()
      return m.id.toLowerCase().includes(q) || (m.name && m.name.toLowerCase().includes(q))
    })
  )

  $effect(() => {
    exampleInput = kind === 'webSearch' ? 'What is the latest news about AI?' : 'https://example.com'
  })

  $effect(() => {
    if (apiKeys && apiKeys.length > 0) {
      const active = apiKeys.find((k) => k.isActive === 1 && k.key)
      if (active) activeApiKey = active.key
    }
  })

  onMount(async () => {
    if (!activeApiKey) {
      try {
        const keys = await api.getApiKeys()
        const active = keys.find((k) => k.isActive === 1 && k.key)
        if (active) activeApiKey = active.key
      } catch {}
    }
  })

  function copyCurl() {
    navigator.clipboard.writeText(exampleCurl)
    copiedCurl = true
    setTimeout(() => { copiedCurl = false }, 2000)
  }

  let defaultResponse = $derived(
    kind === 'webSearch'
      ? `{\n  "results": [\n    { "title": "...", "url": "...", "snippet": "..." }\n  ]\n}`
      : `{\n  "title": "Example Domain",\n  "text": "Hello world..."\n}`
  )

  let exampleCurl = $derived.by(() => {
    const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:20130'
    const key = activeApiKey || 'YOUR_KEY'
    const model = provider.alias || provider.id
    if (kind === 'webSearch') {
      return `curl -X POST ${origin}/v1/search \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${key}" \\\n  -d '{"model":"${model}","query":"${exampleInput}","search_type":"${searchType}","max_results":${maxResults}}'`
    }
    return `curl -X POST ${origin}/v1/web/fetch \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${key}" \\\n  -d '{"model":"${model}","url":"${exampleInput}"}'`
  })

  async function handleTestExample() {
    isTestingExample = true
    exampleResponse = null
    const start = performance.now()
    try {
      const endpoint = kind === 'webSearch' ? '/v1/search' : '/v1/web/fetch'
      const body = kind === 'webSearch'
        ? { model: provider.alias || provider.id, query: exampleInput, search_type: searchType, max_results: maxResults }
        : { model: provider.alias || provider.id, url: exampleInput }
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (activeApiKey) {
        headers['Authorization'] = `Bearer ${activeApiKey}`
      }
      if (selectedConnId) {
        headers['x-router-connection-id'] = selectedConnId
      }
      const res = await fetch(endpoint, {
        method: 'POST',
        headers,
        body: JSON.stringify(body)
      })
      exampleLatency = Math.round(performance.now() - start)
      const data = await res.json()
      exampleResponse = JSON.stringify(data, null, 2)
    } catch (err) {
      exampleLatency = Math.round(performance.now() - start)
      exampleResponse = JSON.stringify({ error: err instanceof Error ? err.message : String(err) }, null, 2)
    } finally {
      isTestingExample = false
    }
  }

  function handleCopyModel(modelId: string) {
    const full = `${provider.alias || provider.id}/${modelId}`
    navigator.clipboard.writeText(full)
    copiedModelId = modelId
    setTimeout(() => { if (copiedModelId === modelId) copiedModelId = null }, 2000)
  }

  async function handleTestModel(modelId: string) {
    testingModelId = modelId
    const full = `${provider.alias || provider.id}/${modelId}`
    const start = performance.now()
    try {
      const res = await api.testModel(full)
      modelTestResults[modelId] = { ok: res.ok, error: res.error, latency: Math.round(performance.now() - start) }
    } catch (err) {
      modelTestResults[modelId] = { ok: false, error: err instanceof Error ? err.message : 'Error', latency: Math.round(performance.now() - start) }
    } finally {
      testingModelId = null
    }
  }
</script>

<div class="flex flex-col gap-6 animate-fade-in">
  <!-- Top Breadcrumbs -->
  <div class="flex items-center gap-2 text-sm text-text-muted">
    <a href="/dashboard/media-providers" onclick={(e) => { e.preventDefault(); onBack() }} class="hover:text-primary transition-colors cursor-pointer">
      Media Providers
    </a>
    <span class="text-text-muted/60">&gt;</span>
    <a href="/dashboard/media-providers/web" onclick={(e) => { e.preventDefault(); onBack() }} class="hover:text-primary transition-colors cursor-pointer">
      {kind === 'webSearch' ? 'Web Search' : kind === 'webFetch' ? 'Web Fetch' : kind}
    </a>
    <span class="text-text-muted/60">&gt;</span>
    <div class="flex items-center gap-1.5 font-semibold text-text-main">
      <img src={icon} alt={provider.name} class="size-4 object-contain rounded" onerror={(e) => { (e.currentTarget as HTMLElement).style.display = 'none' }} />
      <span>{provider.name}</span>
    </div>
  </div>

  {#if kind === 'webSearch' || kind === 'webFetch'}
    <!-- Web Search / Web Fetch Config Card -->
    <div class="bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-soft)] p-6">
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-lg font-semibold text-text-main">{kind === 'webSearch' ? 'Web Search Config' : 'Web Fetch Config'}</h2>
        <a href="https://antigravity.google" target="_blank" rel="noopener noreferrer" class="text-xs text-primary hover:underline inline-flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">open_in_new</span>
          Get API Key
        </a>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2">
        <div class="flex items-center gap-3 min-w-0">
          <span class="text-xs text-text-muted w-28 shrink-0">Mode</span>
          <span class="text-sm text-text-main truncate">chat-completions</span>
        </div>
        <div class="flex items-center gap-3 min-w-0">
          <span class="text-xs text-text-muted w-28 shrink-0">Model</span>
          <span class="text-sm text-text-main truncate font-mono">{provider.id === 'antigravity' ? 'gemini-2.5-flash' : provider.alias || provider.id}</span>
        </div>
        <div class="flex items-center gap-3 min-w-0 sm:col-span-2">
          <span class="text-xs text-text-muted w-28 shrink-0">Free tier</span>
          <span class="text-sm text-text-main truncate">Free — Google Search grounding through an Antigravity OAuth account.</span>
        </div>
      </div>
    </div>

    <!-- Example Card -->
    <div class="bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-soft)] p-6">
      <h2 class="text-lg font-semibold text-text-main mb-4">Example</h2>
      <div class="flex flex-col gap-2.5">
        <!-- Endpoint -->
        <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Endpoint</span>
          <div class="w-full min-w-0 flex-1">
            <span class="px-3 py-1.5 text-sm font-mono text-text-main bg-sidebar rounded-lg truncate block">
              {typeof window !== 'undefined' ? window.location.origin : 'http://localhost:20130'}{kind === 'webSearch' ? '/v1/search' : '/v1/web/fetch'}
            </span>
          </div>
        </div>

        <!-- API Key -->
        <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">API Key</span>
          <div class="w-full min-w-0 flex-1">
            <span class="px-3 py-1.5 text-sm font-mono text-text-main bg-sidebar rounded-lg truncate block">
              {#if activeApiKey}
                {activeApiKey.slice(0, 8) + '•'.repeat(Math.min(20, Math.max(0, activeApiKey.length - 8)))}
              {:else}
                <span class="text-text-muted italic">No key configured</span>
              {/if}
            </span>
          </div>
        </div>

        <!-- Connection selector -->
        <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Connection</span>
          <div class="w-full min-w-0 flex-1">
            <select bind:value={selectedConnId} class="w-full px-3 py-1.5 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main">
              <option value="">Auto (by priority)</option>
              {#each providerConns as conn}
                <option value={conn.id}>{conn.email || conn.name || conn.id.slice(0, 8)}</option>
              {/each}
            </select>
          </div>
        </div>

        <!-- Query / URL -->
        <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">
            {kind === 'webSearch' ? 'Query' : 'URL'}
          </span>
          <div class="w-full min-w-0 flex-1 relative">
            <input
              bind:value={exampleInput}
              class="w-full px-3 py-1.5 pr-7 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main font-mono"
              placeholder={kind === 'webSearch' ? 'What is the latest news about AI?' : 'https://example.com'}
            />
            {#if exampleInput}
              <button
                type="button"
                onclick={() => (exampleInput = '')}
                class="absolute right-2 top-1/2 -translate-y-1/2 text-text-muted hover:text-primary transition-colors cursor-pointer"
              >
                <span class="material-symbols-outlined text-[14px]">close</span>
              </button>
            {/if}
          </div>
        </div>

        {#if kind === 'webSearch'}
          <!-- Type -->
          <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
            <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Type</span>
            <div class="w-full min-w-0 flex-1">
              <select bind:value={searchType} class="w-full px-3 py-1.5 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main">
                <option value="web">web</option>
                <option value="news">news</option>
              </select>
            </div>
          </div>

          <!-- Max results -->
          <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
            <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Max results</span>
            <div class="w-full min-w-0 flex-1">
              <input type="number" min="1" max="100" bind:value={maxResults} class="w-full px-3 py-1.5 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main" />
            </div>
          </div>

          <!-- Country -->
          <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
            <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Country</span>
            <div class="w-full min-w-0 flex-1">
              <input type="text" bind:value={country} class="w-full px-3 py-1.5 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main" />
            </div>
          </div>

          <!-- Language -->
          <div class="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
            <span class="w-full text-xs font-medium text-text-muted sm:w-24 sm:shrink-0">Language</span>
            <div class="w-full min-w-0 flex-1">
              <input type="text" bind:value={language} class="w-full px-3 py-1.5 text-sm border border-border rounded-lg bg-background focus:outline-none focus:border-primary text-text-main" />
            </div>
          </div>
        {/if}

        <!-- REQUEST Section -->
        <div class="mt-1">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mb-1.5">
            <span class="text-xs font-semibold text-text-muted uppercase tracking-wider">Request</span>
            <div class="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center">
              <button type="button" onclick={copyCurl} class="inline-flex items-center gap-1 text-xs text-text-muted hover:text-primary transition-colors cursor-pointer mr-2">
                <span class="material-symbols-outlined text-[14px]">content_copy</span>
                {copiedCurl ? 'Copied' : 'Copy'}
              </button>
              <button
                type="button"
                onclick={handleTestExample}
                disabled={isTestingExample}
                class="flex w-full sm:w-auto items-center justify-center gap-1.5 px-3 py-1 rounded-lg bg-primary text-white text-xs font-medium hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
              >
                <span class="material-symbols-outlined text-[14px]">play_arrow</span>
                {isTestingExample ? 'Running...' : 'Run'}
              </button>
            </div>
          </div>
          <pre class="bg-sidebar rounded-lg px-3 py-2.5 text-xs font-mono text-text-main overflow-x-auto whitespace-pre-wrap break-all">{exampleCurl}</pre>
        </div>

        <!-- RESPONSE Section -->
        <div>
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mb-1.5">
            <span class="text-xs font-semibold text-text-muted uppercase tracking-wider">
              Response {exampleLatency ? `(${exampleLatency}ms)` : ''}
            </span>
          </div>
          <pre class="bg-sidebar rounded-lg px-3 py-2.5 text-xs font-mono text-text-main overflow-x-auto whitespace-pre-wrap break-all opacity-70">{exampleResponse || defaultResponse}</pre>
        </div>
      </div>
    </div>
  {:else}
    <!-- Models Section for non-search/fetch media kinds -->
    <div class="flex flex-col gap-3">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <h2 class="text-sm font-semibold uppercase tracking-wider text-text-muted flex items-center gap-2">
          <span class="material-symbols-outlined text-brand-500">show_chart</span>
          {kind.toUpperCase()} Models ({mediaModels.length})
        </h2>
        <div class="relative w-full sm:w-64">
          <input
            type="text"
            bind:value={searchQuery}
            placeholder="Filter models..."
            class="w-full pl-3 pr-3 py-1 text-xs rounded-lg bg-surface border border-border text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500"
          />
        </div>
      </div>

      {#if filteredModels.length === 0}
        <Card class="p-8 text-center text-text-muted text-xs border-dashed">
          <p>No {kind} models found{searchQuery ? ` matching "${searchQuery}"` : ''}.</p>
        </Card>
      {:else}
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {#each filteredModels as model (model.id)}
            <MediaModelCard
              {model}
              {kind}
              isCopied={copiedModelId === model.id}
              isTesting={testingModelId === model.id}
              testResult={modelTestResults[model.id]}
              onCopy={() => handleCopyModel(model.id)}
              onTest={() => handleTestModel(model.id)}
            />
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>
