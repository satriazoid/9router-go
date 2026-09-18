<script lang="ts">
  import { onMount } from 'svelte'
  import type { ActiveRequestItem } from './types'
  import { getIconPath } from '../connections/types'

  interface ProviderNodeItem {
    id: string
    name: string
    color?: string
    type?: string
  }

  interface Props {
    providers?: ProviderNodeItem[]
    activeRequests?: ActiveRequestItem[]
    lastProvider?: string
    errorProvider?: string
    onRefresh?: () => void
  }

  let {
    providers = [],
    activeRequests = [],
    lastProvider = '',
    errorProvider = '',
    onRefresh
  }: Props = $props()

  // Default fallback providers if none connected
  const FALLBACK_PROVIDERS: ProviderNodeItem[] = [
    { id: 'antigravity', name: 'Antigravity', color: '#F59E0B' },
    { id: 'opencode', name: 'OpenCode Free', color: '#3B82F6' },
    { id: 'nvidia', name: 'NVIDIA NIM', color: '#76B900' },
    { id: 'freebuff', name: 'Freebuff', color: '#10B981' },
    { id: 'openrouter', name: 'OpenRouter', color: '#6366F1' },
    { id: 'clinepass', name: 'ClinePass', color: '#8B5CF6' }
  ]

  let displayProviders = $derived(
    providers.length > 0 ? providers.slice(0, 14) : FALLBACK_PROVIDERS
  )

  let activeProviderIds = $derived(
    new Set(
      activeRequests
        .map((r) => r.provider?.toLowerCase())
        .filter((p): p is string => Boolean(p))
    )
  )

  let activeCount = $derived(activeRequests.reduce((sum, r) => sum + (r.count || 1), 0))

  // Container sizing and fitView state
  let containerEl = $state<HTMLDivElement | null>(null)
  let containerWidth = $state(800)
  let containerHeight = $state(480)

  // Zoom & Pan transforms
  let zoom = $state(0.85)
  let panX = $state(0)
  let panY = $state(0)
  let isDragging = $state(false)
  let dragStart = { x: 0, y: 0 }

  // Track image load errors to fallback to stylish initials
  let imageErrors = $state<Record<string, boolean>>({})

  // Upstream exact node radius formula:
  // s = providers.length
  // a = Math.max(320, 204 * s / (2 * Math.PI)) // rx
  // i = Math.max(200, 0.55 * a)                 // ry
  let geometry = $derived.by(() => {
    const s = displayProviders.length
    const rx = Math.max(320, (204 * s) / (2 * Math.PI))
    const ry = Math.max(200, 0.55 * rx)

    const nodes = displayProviders.map((p, idx) => {
      const angle = -Math.PI / 2 + (2 * Math.PI * idx) / s
      const x = rx * Math.cos(angle)
      const y = ry * Math.sin(angle)

      const pid = p.id.toLowerCase()
      const isActive =
        activeProviderIds.has(pid) ||
        lastProvider.toLowerCase() === pid ||
        activeProviderIds.has(p.name.toLowerCase())
      const isError = errorProvider.toLowerCase() === pid

      // Edge handle coordinates based on angle
      let sourceX = 0
      let sourceY = 0
      let targetX = x
      let targetY = y
      let isVertical = false

      if (
        Math.abs(angle + Math.PI / 2) < Math.PI / 4 ||
        Math.abs(angle - (3 * Math.PI) / 2) < Math.PI / 4
      ) {
        // Top quadrant
        sourceX = 0
        sourceY = -22
        targetX = x
        targetY = y + 26
        isVertical = true
      } else if (Math.abs(angle - Math.PI / 2) < Math.PI / 4) {
        // Bottom quadrant
        sourceX = 0
        sourceY = 22
        targetX = x
        targetY = y - 26
        isVertical = true
      } else if (angle > -Math.PI / 2 && angle < Math.PI / 2) {
        // Right quadrant
        sourceX = 65
        sourceY = 0
        targetX = x - 75
        targetY = y
        isVertical = false
      } else {
        // Left quadrant
        sourceX = -65
        sourceY = 0
        targetX = x + 75
        targetY = y
        isVertical = false
      }

      const path = isVertical
        ? `M ${sourceX} ${sourceY} C ${sourceX} ${(sourceY + targetY) / 2}, ${targetX} ${(sourceY + targetY) / 2}, ${targetX} ${targetY}`
        : `M ${sourceX} ${sourceY} C ${(sourceX + targetX) / 2} ${sourceY}, ${(sourceX + targetX) / 2} ${targetY}, ${targetX} ${targetY}`

      const textIcon = (p.name || p.id || '?').slice(0, 2).toUpperCase()

      return {
        ...p,
        x,
        y,
        isActive,
        isError,
        path,
        textIcon,
        color: p.color || '#6b7280'
      }
    })

    return { rx, ry, nodes }
  })

  function fitView() {
    if (!containerWidth || !containerHeight) return
    const requiredWidth = 2 * geometry.rx + 220
    const requiredHeight = 2 * geometry.ry + 120
    const scaleX = (containerWidth - 32) / requiredWidth
    const scaleY = (containerHeight - 32) / requiredHeight
    zoom = Math.min(1.05, Math.max(0.3, Math.min(scaleX, scaleY)))
    panX = 0
    panY = 0
  }

  function handlePointerDown(e: PointerEvent) {
    if ((e.target as HTMLElement).closest('button')) return
    isDragging = true
    dragStart = { x: e.clientX - panX, y: e.clientY - panY }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging) return
    panX = e.clientX - dragStart.x
    panY = e.clientY - dragStart.y
  }

  function handlePointerUp(e: PointerEvent) {
    if (isDragging) {
      isDragging = false
      try {
        ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
      } catch {}
    }
  }

  function handleWheel(e: WheelEvent) {
    e.preventDefault()
    const zoomFactor = e.deltaY < 0 ? 1.08 : 0.92
    zoom = Math.min(2.2, Math.max(0.25, zoom * zoomFactor))
  }

  onMount(() => {
    if (!containerEl) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width || 800
        containerHeight = entry.contentRect.height || 480
        fitView()
      }
    })
    observer.observe(containerEl)
    fitView()
    return () => observer.disconnect()
  })
</script>

<div
  bind:this={containerEl}
  role="region"
  aria-label="Provider topology map"
  class="h-[320px] w-full min-w-0 rounded-lg border border-border bg-bg-subtle/30 sm:h-[480px] relative overflow-hidden select-none cursor-grab active:cursor-grabbing"
  onpointerdown={handlePointerDown}
  onpointermove={handlePointerMove}
  onpointerup={handlePointerUp}
  onpointercancel={handlePointerUp}
  onwheel={handleWheel}
>
  <!-- Interactive Viewport Layer (centered at 0, 0) -->
  <div
    class="absolute inset-0 pointer-events-none"
    style="transform: translate({containerWidth / 2 + panX}px, {containerHeight / 2 + panY}px) scale({zoom}); transform-origin: 0 0;"
  >
    <!-- SVG Layer for Bezier Edges -->
    <svg class="absolute inset-0 overflow-visible pointer-events-none" style="transform: translate(0, 0);">
      <defs>
        {#each geometry.nodes as node (node.id)}
          <filter id="topo-electric-{node.id}" x="-40%" y="-40%" width="180%" height="180%">
            <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" seed="2" result="noise">
              <animate attributeName="baseFrequency" values="0.8;1.4;0.8" dur="0.25s" repeatCount="indefinite" />
            </feTurbulence>
            <feDisplacementMap in="SourceGraphic" in2="noise" scale="3.5" xChannelSelector="R" yChannelSelector="G" />
          </filter>
        {/each}
      </defs>

      <!-- Edges connecting router to provider nodes -->
      {#each geometry.nodes as node (node.id)}
        {#if node.isActive}
          <!-- Active Electric Plasma Edge -->
          <g class="topology-edge-electric">
            <path d={node.path} fill="none" stroke="rgba(245, 158, 11, 0.25)" stroke-width="6" />
            <path
              d={node.path}
              fill="none"
              stroke="#f59e0b"
              stroke-width="2"
              filter="url(#topo-electric-{node.id})"
            />
            {#each [0, 1, 2] as r}
              <circle r="3" fill="#f59e0b">
                <animateMotion
                  dur="{0.8 + 0.1 * r}s"
                  repeatCount="indefinite"
                  path={node.path}
                  begin="{0.2 * r}s"
                />
              </circle>
            {/each}
          </g>
        {:else}
          <!-- Inactive Subtle Edge -->
          <path
            d={node.path}
            fill="none"
            stroke="var(--color-border)"
            stroke-width="1"
            opacity="0.3"
          />
        {/if}
      {/each}
    </svg>

    <!-- HTML Router Node (Center 0, 0) -->
    <div
      class="absolute z-10 flex items-center justify-center px-5 py-3 rounded-xl border-2 min-w-[130px] {activeCount > 0 ? 'topology-router-core border-yellow-300 bg-gradient-to-br from-primary/30 via-yellow-400/20 to-cyan-400/25' : 'border-primary bg-primary/5 shadow-md'} pointer-events-auto"
      style="left: 0px; top: 0px; transform: translate(-50%, -50%);"
    >
      <img
        src="/favicon.svg"
        alt="9router-go"
        class="w-6 h-6 mr-2 object-contain"
        loading="lazy"
        decoding="async"
      />
      <span class="text-sm font-bold text-primary">9router-go</span>
    </div>

    <!-- HTML Provider Nodes -->
    {#each geometry.nodes as node (node.id)}
      <div
        class="absolute flex items-center gap-2.5 px-4 py-2.5 rounded-lg border-2 transition-all duration-300 bg-bg shadow-sm pointer-events-auto"
        style="left: {node.x}px; top: {node.y}px; transform: translate(-50%, -50%); border-color: {node.isActive ? node.color : 'var(--color-border)'}; box-shadow: {node.isActive ? `0 0 16px ${node.color}40` : 'none'}; min-width: 150px;"
      >
        <div
          class="w-8 h-8 rounded-md flex items-center justify-center shrink-0"
          style="background-color: {node.color}15;"
        >
          {#if !imageErrors[node.id]}
            <img
              src={getIconPath(node.id)}
              alt={node.name}
              class="w-6 h-6 rounded-sm object-contain"
              onerror={() => {
                imageErrors = { ...imageErrors, [node.id]: true }
              }}
              loading="lazy"
              decoding="async"
            />
          {:else}
            <span class="text-sm font-bold" style="color: {node.color};">
              {node.textIcon}
            </span>
          {/if}
        </div>
        <span
          class="text-base font-medium truncate text-text-main max-w-[140px]"
          title={node.name}
        >
          {node.name}
        </span>
      </div>
    {/each}
  </div>

  <!-- Bottom-left React Flow style controls -->
  <div class="absolute bottom-4 left-4 z-20 flex flex-col rounded-md border border-border bg-surface/90 shadow-md overflow-hidden backdrop-blur">
    <button
      type="button"
      onclick={() => (zoom = Math.min(2.5, zoom * 1.2))}
      class="p-1.5 text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors border-b border-border cursor-pointer flex items-center justify-center"
      title="Zoom In"
      aria-label="Zoom In"
    >
      <span class="material-symbols-outlined text-[16px]">add</span>
    </button>
    <button
      type="button"
      onclick={() => (zoom = Math.max(0.2, zoom / 1.2))}
      class="p-1.5 text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors border-b border-border cursor-pointer flex items-center justify-center"
      title="Zoom Out"
      aria-label="Zoom Out"
    >
      <span class="material-symbols-outlined text-[16px]">remove</span>
    </button>
    <button
      type="button"
      onclick={fitView}
      class="p-1.5 text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer flex items-center justify-center"
      title="Fit View"
      aria-label="Fit View"
    >
      <span class="material-symbols-outlined text-[16px]">crop_free</span>
    </button>
  </div>
</div>
