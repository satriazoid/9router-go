<script lang="ts">
  import { liveActivity } from '@/lib/useLiveActivity'
  import { onMount, onDestroy } from 'svelte'

  let nowMs = Date.now()
  let clock: ReturnType<typeof setInterval> | null = null

  onMount(() => {
    liveActivity.start()
    // Lokal 100ms clock agar timer tetap bergerak
    clock = setInterval(() => { nowMs = Date.now() }, 100)
  })

  onDestroy(() => {
    liveActivity.stop()
    if (clock) clearInterval(clock)
  })

  // Derived dari store
  const snapshot = $derived(liveActivity.snapshot)
  const stale = $derived(liveActivity.stale)
  const history = $derived(liveActivity.history)

  const active = $derived(snapshot.active)
  const waiting = $derived(snapshot.waiting)
  const inflight = $derived(snapshot.inflight)
  const streams = $derived(snapshot.streams)
  const errors = $derived(snapshot.errors)
  const rps = $derived(snapshot.rps)
  const tps = $derived(snapshot.tps)

  // Format helper
  function fmtMs(ms: number): string {
    if (!Number.isFinite(ms) || ms < 0) return '0ms'
    if (ms < 1000) return `${Math.round(ms)}ms`
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
    return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
  }

  function fmtRate(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return '0'
    if (n < 1000) return n < 100 ? n.toFixed(1) : String(Math.round(n))
    if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`
    return `${(n / 1_000_000).toFixed(1)}M`
  }
</script>

<!-- Desktop Live Activity Widget — mirip LiveActivityAnimation jrouters -->
<div class="desktop-widget bg-gray-900 text-gray-100 border border-gray-600 rounded-lg overflow-hidden max-w-sm mx-auto m-4">
  <!-- Header -->
  <div class="p-4 border-b border-gray-600 flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="h-2 w-2 rounded-full bg-green-500" />
      <span class="text-sm font-medium">9Router Go</span>
    </div>
    <span class="text-xs text-gray-400">Live</span>
  </div>

  <!-- Stats -->
  <div class="p-4 grid grid-cols-2 gap-2">
    <div>
      <div class="text-green-400 text-sm font-bold">{rps.toFixed(1)}</div>
      <div class="text-xs text-gray-300">req/s</div>
    </div>
    <div>
      <div class="text-green-400 text-sm font-bold">{fmtRate(tps)}</div>
      <div class="text-xs text-gray-300">tok/s</div>
    </div>
  </div>

  <!-- Throughput chart (mini) -->
  <div class="p-2 h-6 bg-gray-800 rounded-t">
    {#each [rps, rps*0.8, rps*0.6, rps*0.4] as val}
      <div class="h-0.5 bg-green-500/30 transition-[height] duration-200" style={{ height: `${Math.max(2, (val / Math.max(1, rps)) * 20)}px` }} title="${val.toFixed(1)} req/s"></div>
    {/each}
  </div>

  <!-- Pipeline indicator -->
  {active > 0 && !stale ? (
    <div class="h-1 bg-green-500 w-full rounded" style={{ animation: 'live-sweep 1.6s cubic-bezier(0.4, 0, 0.2, 1) infinite' }} />
  ) : (
    <div class="h-1 bg-gray-600 w-full rounded" />
  )}

  <!-- Aggregate -->
  <div class="p-2 text-xs text-gray-300 flex flex-wrap gap-1">
    {active > 0 && `<span class="text-green-300">active: ${active}</span>`}
    {inflight > 0 && `<span class="text-gray-300">inflight: ${inflight}</span>`}
    {errors > 0 && `<span class="text-red-300">errors: ${errors}</span>`}
  </div>

  <!-- Rows jika ada data -->
  {active > 0 ? (
    <div class="p-2 text-xs text-gray-200 space-y-1">
      {[{ status: 'running' }, { status: 'waiting' }].slice(0, 4).map((s) => (
        <div key={s.status} class="flex items-center gap-1">
          <span class="h-1 w-1 rounded-full bg-green-500" />
          <span>{s.status}</span>
        </div>
      ))}
    </div>
  ) : (
    <div class="p-2 text-center text-gray-400">No active requests</div>
  )}
</div>

<style>
@keyframes live-sweep {
  0% { left: 0; width: 0; }
  100% { left: 100%; width: 100%; }
}
</style>
</script>