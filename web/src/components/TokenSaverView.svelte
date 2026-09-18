<script lang="ts">
  import { onMount } from 'svelte'
  import {
    AlertCircle,
    Check,
    Copy,
    ExternalLink,
    Loader2,
    PiggyBank,
    RefreshCw,
    X,
    Zap
  } from 'lucide-svelte'
  import Card from '../lib/ui/Card.svelte'
  import Toggle from '../lib/ui/Toggle.svelte'
  import { api, type Settings } from '../api/client'

  interface Props {
    settings?: Settings
    onRefresh?: () => void
  }

  let {
    settings = {},
    onRefresh
  }: Props = $props()

  // State
  let rtkEnabled = $state(false)
  let headroomEnabled = $state(false)
  let headroomUrl = $state('http://localhost:8787')
  let headroomTimeoutMs = $state(3000)
  let cavemanEnabled = $state(false)
  let cavemanLevel = $state('full')
  let ponytailEnabled = $state(false)
  let ponytailLevel = $state('full')

  // Headroom status & setup modal
  let headroomStatus = $state<'checking' | 'reachable' | 'unreachable'>('checking')
  let isHeadroomModalOpen = $state(false)
  let isRecheckingHeadroom = $state(false)
  let copiedInstallCmd = $state(false)
  let isSavingSettings = $state(false)

  // Caveman levels definitions
  const cavemanLevels = [
    { id: 'lite', label: 'Lite', desc: 'Drop filler, keep grammar' },
    { id: 'full', label: 'Full', desc: 'Drop articles, fragments OK' },
    { id: 'ultra', label: 'Ultra', desc: 'Telegraphic, max compression' },
    { id: 'wenyan-lite', label: '文 Lite', desc: 'Classical Chinese, light compression' },
    { id: 'wenyan', label: '文 Full', desc: 'Maximum 文言文, 80-90% reduction' },
    { id: 'wenyan-ultra', label: '文 Ultra', desc: 'Extreme classical compression' },
  ]

  // Ponytail levels definitions
  const ponytailLevels = [
    { id: 'lite', label: 'Lite', desc: 'Build asked, name lazier option' },
    { id: 'full', label: 'Full', desc: 'Ladder enforced: stdlib/native first' },
    { id: 'ultra', label: 'Ultra', desc: 'YAGNI extremist, deletion first' },
  ]

  let currentCavemanDesc = $derived(
    cavemanLevels.find((l) => l.id === cavemanLevel)?.desc || 'Drop articles, fragments OK'
  )

  let currentPonytailDesc = $derived(
    ponytailLevels.find((l) => l.id === ponytailLevel)?.desc || 'Ladder enforced: stdlib/native first'
  )

  // Sync props
  $effect(() => {
    if (settings) {
      if (typeof settings.rtkEnabled === 'boolean') rtkEnabled = settings.rtkEnabled
      if (typeof settings.headroomEnabled === 'boolean') headroomEnabled = settings.headroomEnabled
      if (typeof settings.headroomUrl === 'string' && settings.headroomUrl) headroomUrl = settings.headroomUrl
      if (typeof settings.headroomTimeoutMs === 'number' && settings.headroomTimeoutMs > 0) {
        headroomTimeoutMs = settings.headroomTimeoutMs
      }
      if (typeof settings.cavemanEnabled === 'boolean') cavemanEnabled = settings.cavemanEnabled
      if (typeof settings.cavemanLevel === 'string' && settings.cavemanLevel) cavemanLevel = settings.cavemanLevel
      if (typeof settings.ponytailEnabled === 'boolean') ponytailEnabled = settings.ponytailEnabled
      if (typeof settings.ponytailLevel === 'string' && settings.ponytailLevel) ponytailLevel = settings.ponytailLevel
    }
  })

  // Load initial settings & headroom status
  async function loadInitialData() {
    try {
      const s = await api.getSettings()
      if (s) {
        rtkEnabled = !!s.rtkEnabled
        headroomEnabled = !!s.headroomEnabled
        if (s.headroomUrl) headroomUrl = s.headroomUrl
        if (typeof s.headroomTimeoutMs === 'number' && s.headroomTimeoutMs > 0) {
          headroomTimeoutMs = s.headroomTimeoutMs
        }
        cavemanEnabled = !!s.cavemanEnabled
        if (s.cavemanLevel) cavemanLevel = s.cavemanLevel
        ponytailEnabled = !!s.ponytailEnabled
        if (s.ponytailLevel) ponytailLevel = s.ponytailLevel
      }
    } catch {
      // silent
    }
    await checkHeadroomStatus()
  }

  async function checkHeadroomStatus() {
    headroomStatus = 'checking'
    isRecheckingHeadroom = true
    try {
      const res = await api.getHeadroomStatus()
      if (res.reachable || res.running) {
        headroomStatus = 'reachable'
      } else {
        headroomStatus = 'unreachable'
      }
    } catch {
      headroomStatus = 'unreachable'
    } finally {
      isRecheckingHeadroom = false
    }
  }

  onMount(() => {
    loadInitialData()
  })

  // Persist settings update
  async function updateSettingField(updates: Partial<Settings>) {
    isSavingSettings = true
    try {
      await api.updateSettings(updates)
      onRefresh?.()
    } catch (err) {
      alert(`Failed to save settings: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isSavingSettings = false
    }
  }

  function handleToggleRTK(value: boolean) {
    rtkEnabled = value
    updateSettingField({ rtkEnabled: value })
  }

  function handleToggleHeadroom(value: boolean) {
    headroomEnabled = value
    updateSettingField({ headroomEnabled: value, headroomUrl })
  }

  function handleToggleCaveman(value: boolean) {
    cavemanEnabled = value
    updateSettingField({ cavemanEnabled: value })
  }

  function handleSelectCavemanLevel(levelId: string) {
    cavemanLevel = levelId
    updateSettingField({ cavemanLevel: levelId })
  }

  function handleTogglePonytail(value: boolean) {
    ponytailEnabled = value
    updateSettingField({ ponytailEnabled: value })
  }

  function handleSelectPonytailLevel(levelId: string) {
    ponytailLevel = levelId
    updateSettingField({ ponytailLevel: levelId })
  }

  function copyInstallCommand() {
    navigator.clipboard.writeText('pip install "headroom-ai[proxy]"')
    copiedInstallCmd = true
    setTimeout(() => (copiedInstallCmd = false), 2000)
  }
</script>

<div class="flex flex-col gap-6">
  <!-- PAGE HEADER -->
  <div class="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
    <div class="space-y-1">
      <div class="flex items-center gap-2">
        <div class="p-2 rounded-lg bg-brand-500/10 text-brand-500">
          <PiggyBank class="w-5 h-5" />
        </div>
        <div>
          <h1 class="font-headline text-2xl sm:text-3xl font-bold text-text-main tracking-tight flex items-center gap-2">
            Token Saver
          </h1>
          <p class="font-body text-xs sm:text-sm text-text-muted">
            Compress prompts and outputs to save tokens
          </p>
        </div>
      </div>
    </div>
  </div>

  <!-- MAIN CARD: Token Saver Engines -->
  <Card padding="md" class="space-y-6">
    <!-- Card Title -->
    <div class="flex items-center gap-2 pb-2 border-b border-border">
      <Zap class="w-5 h-5 text-brand-500" />
      <h2 class="text-base font-bold text-text-main">
        Token Saver
      </h2>
    </div>

    <!-- 1. Compress tool output (RTK) -->
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex items-center gap-2">
          <p class="font-semibold text-sm text-text-main">Compress tool output</p>
          <a
            href="https://github.com/rtk-ai/rtk"
            target="_blank"
            rel="noreferrer"
            class="text-xs text-brand-500 underline hover:opacity-80 font-mono inline-flex items-center gap-0.5"
          >
            <span>(RTK)</span>
            <ExternalLink class="w-3 h-3" />
          </a>
        </div>
        <p class="text-xs text-text-muted leading-relaxed">
          git/grep/ls/tree/logs → 60-90% fewer input tokens
        </p>
      </div>

      <div class="shrink-0 pt-0.5">
        <Toggle
          checked={rtkEnabled}
          label="Compress tool output (RTK)"
          onChange={handleToggleRTK}
        />
      </div>
    </div>

    <!-- 2. Compress context (Headroom) -->
    <div class="flex items-start justify-between gap-4 pt-4 border-t border-border/50">
      <div class="min-w-0 flex-1 space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <p class="font-semibold text-sm text-text-main">Compress context</p>
          <a
            href="https://github.com/chopratejas/headroom"
            target="_blank"
            rel="noreferrer"
            class="text-xs text-brand-500 underline hover:opacity-80 font-mono inline-flex items-center gap-0.5"
          >
            <span>(Headroom)</span>
            <ExternalLink class="w-3 h-3" />
          </a>

          <!-- Status badge -->
          <span
            class="text-[11px] font-semibold px-2 py-0.5 rounded border {headroomStatus === 'reachable'
              ? 'bg-success/10 text-success border-success/20'
              : headroomStatus === 'checking'
                ? 'bg-surface-2 text-text-muted border-border'
                : 'bg-warning/10 text-warning border-warning/20'}"
          >
            {#if headroomStatus === 'checking'}
              Checking...
            {:else if headroomStatus === 'reachable'}
              Reachable
            {:else}
              Not reachable
            {/if}
          </span>

          <!-- Setup / Manage button -->
          <button
            type="button"
            onclick={() => (isHeadroomModalOpen = true)}
            class="text-xs text-brand-500 font-semibold underline hover:opacity-80 cursor-pointer"
          >
            {headroomStatus === 'reachable' ? 'Manage' : 'Setup'}
          </button>
        </div>

        <p class="text-xs text-text-muted leading-relaxed">
          Compress prompts via /v1/compress before routing to the model
        </p>
      </div>

      <div class="shrink-0 pt-0.5">
        <Toggle
          checked={headroomEnabled}
          label="Compress context (Headroom)"
          onChange={handleToggleHeadroom}
        />
      </div>
    </div>

    <!-- 3. Compress LLM output (Caveman) -->
    <div class="flex items-start justify-between gap-4 pt-4 border-t border-border/50 flex-wrap">
      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex items-center gap-2">
          <p class="font-semibold text-sm text-text-main">Compress LLM output</p>
          <a
            href="https://github.com/JuliusBrussee/caveman"
            target="_blank"
            rel="noreferrer"
            class="text-xs text-brand-500 underline hover:opacity-80 font-mono inline-flex items-center gap-0.5"
          >
            <span>(Caveman)</span>
            <ExternalLink class="w-3 h-3" />
          </a>
        </div>
        <p class="text-xs text-text-muted leading-relaxed">
          Terse-style system prompt → ~65% fewer output tokens (up to 87%)
        </p>
      </div>

      <div class="flex flex-col sm:flex-row items-end sm:items-center gap-3 shrink-0">
        {#if cavemanEnabled}
          <div class="flex flex-col items-end gap-1.5">
            <div class="flex items-center gap-1.5 flex-wrap">
              {#each cavemanLevels as lvl (lvl.id)}
                <button
                  type="button"
                  onclick={() => handleSelectCavemanLevel(lvl.id)}
                  title={lvl.desc}
                  class="px-2.5 py-1 rounded-md text-xs font-semibold border transition-all cursor-pointer {cavemanLevel === lvl.id
                    ? 'bg-brand-500 text-white border-brand-500 shadow-sm'
                    : 'bg-surface-2 border-border text-text-muted hover:bg-surface-3 hover:text-text-main'}"
                >
                  {lvl.label}
                </button>
              {/each}
            </div>
            <p class="text-[11px] text-brand-500 font-medium">
              {currentCavemanDesc}
            </p>
          </div>
        {/if}

        <Toggle
          checked={cavemanEnabled}
          label="Caveman"
          onChange={handleToggleCaveman}
        />
      </div>
    </div>

    <!-- 4. Lazy senior dev (Ponytail) -->
    <div class="flex items-start justify-between gap-4 pt-4 border-t border-border/50 flex-wrap">
      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex items-center gap-2">
          <p class="font-semibold text-sm text-text-main">Lazy senior dev</p>
          <a
            href="https://github.com/DietrichGebert/ponytail"
            target="_blank"
            rel="noreferrer"
            class="text-xs text-brand-500 underline hover:opacity-80 font-mono inline-flex items-center gap-0.5"
          >
            <span>(Ponytail)</span>
            <ExternalLink class="w-3 h-3" />
          </a>
        </div>
        <p class="text-xs text-text-muted leading-relaxed">
          Bias the model toward minimal code: YAGNI, reuse stdlib, deletion over addition
        </p>
      </div>

      <div class="flex flex-col sm:flex-row items-end sm:items-center gap-3 shrink-0">
        {#if ponytailEnabled}
          <div class="flex flex-col items-end gap-1.5">
            <div class="flex items-center gap-1.5 flex-wrap">
              {#each ponytailLevels as lvl (lvl.id)}
                <button
                  type="button"
                  onclick={() => handleSelectPonytailLevel(lvl.id)}
                  title={lvl.desc}
                  class="px-2.5 py-1 rounded-md text-xs font-semibold border transition-all cursor-pointer {ponytailLevel === lvl.id
                    ? 'bg-brand-500 text-white border-brand-500 shadow-sm'
                    : 'bg-surface-2 border-border text-text-muted hover:bg-surface-3 hover:text-text-main'}"
                >
                  {lvl.label}
                </button>
              {/each}
            </div>
            <p class="text-[11px] text-brand-500 font-medium">
              {currentPonytailDesc}
            </p>
          </div>
        {/if}

        <Toggle
          checked={ponytailEnabled}
          label="Ponytail"
          onChange={handleTogglePonytail}
        />
      </div>
    </div>
  </Card>
</div>

<!-- MODAL: Headroom Setup & Configuration -->
{#if isHeadroomModalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-border">
        <h3 class="text-base font-bold text-text-main">
          {headroomStatus === 'reachable' ? 'Headroom' : 'Setup Headroom'}
        </h3>
        <button
          type="button"
          onclick={() => (isHeadroomModalOpen = false)}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <div class="space-y-3 text-xs">
        <!-- Status Indicator -->
        <div class="flex items-center justify-between p-3 rounded-xl bg-surface-2 border border-border">
          <span class="font-semibold text-text-main">Status</span>
          <span
            class="font-semibold {headroomStatus === 'reachable'
              ? 'text-success'
              : headroomStatus === 'checking'
                ? 'text-text-muted'
                : 'text-warning'}"
          >
            {#if headroomStatus === 'checking'}
              Checking...
            {:else if headroomStatus === 'reachable'}
              Reachable
            {:else}
              Not reachable
            {/if}
          </span>
        </div>

        <!-- Dashboard Link if reachable -->
        {#if headroomStatus === 'reachable'}
          <a
            href="{headroomUrl}/dashboard"
            target="_blank"
            rel="noreferrer"
            class="flex items-center justify-center gap-1.5 w-full py-2 px-3 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border text-xs font-semibold text-text-main transition"
          >
            <span>Open Headroom Dashboard</span>
            <ExternalLink class="w-3.5 h-3.5" />
          </a>
        {/if}

        <!-- Proxy URL Input -->
        <div class="space-y-1">
          <label for="headroom-url-input" class="block font-semibold text-text-main">
            Proxy URL
          </label>
          <input
            id="headroom-url-input"
            type="text"
            bind:value={headroomUrl}
            onblur={() => updateSettingField({ headroomUrl })}
            placeholder="http://localhost:8787"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
          />
          <p class="text-[11px] text-text-subtle">
            Use a local proxy for Start/Stop, or an external Docker sidecar like http://headroom:8787.
          </p>
        </div>

        <!-- Timeout Input -->
        <div class="space-y-1">
          <label for="headroom-timeout-input" class="block font-semibold text-text-main">
            Timeout (ms)
          </label>
          <input
            id="headroom-timeout-input"
            type="number"
            bind:value={headroomTimeoutMs}
            onblur={() => updateSettingField({ headroomTimeoutMs })}
            placeholder="3000"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
          />
          <p class="text-[11px] text-text-subtle">
            Request timeout in milliseconds. Defaults to 3000 ms.
          </p>
        </div>

        <!-- Local Installation Snippet if not reachable -->
        {#if headroomStatus !== 'reachable'}
          <div class="space-y-1.5 pt-1">
            <p class="font-semibold text-text-main">Install then click Start:</p>
            <div class="flex items-center gap-2">
              <code class="flex-1 px-3 py-2 rounded-lg bg-bg border border-border text-[11px] font-mono text-text-muted select-all">
                pip install "headroom-ai[proxy]"
              </code>
              <button
                type="button"
                onclick={copyInstallCommand}
                class="px-3 py-2 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border text-xs font-semibold text-text-main transition cursor-pointer flex items-center gap-1"
              >
                {#if copiedInstallCmd}
                  <Check class="w-3.5 h-3.5 text-success" />
                  <span>Copied</span>
                {:else}
                  <Copy class="w-3.5 h-3.5" />
                  <span>Copy</span>
                {/if}
              </button>
            </div>
            <p class="text-[11px] text-text-subtle">
              Python ≥ 3.10 required for local managed mode. Install Python first, or use an external proxy URL.
            </p>
          </div>
        {/if}
      </div>

      <div class="flex gap-2 pt-2">
        <button
          type="button"
          onclick={checkHeadroomStatus}
          disabled={isRecheckingHeadroom}
          class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border text-text-main font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5"
        >
          {#if isRecheckingHeadroom}
            <Loader2 class="w-3.5 h-3.5 animate-spin" />
            <span>Rechecking...</span>
          {:else}
            <RefreshCw class="w-3.5 h-3.5" />
            <span>Recheck</span>
          {/if}
        </button>
        <button
          type="button"
          onclick={() => (isHeadroomModalOpen = false)}
          class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer"
        >
          Done
        </button>
      </div>
    </div>
  </div>
{/if}
