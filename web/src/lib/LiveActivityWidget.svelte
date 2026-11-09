<script lang="ts">
  import { liveActivity, type LiveSession } from '@/lib/useLiveActivity'
  import { onMount, onDestroy } from 'svelte'

  // ---------- helpers lokal (tanpa dependensi eksternal rusak) ----------
  let nowMs = Date.now()
  let clock: ReturnType<typeof setInterval> | null = null

  onMount(() => {
    liveActivity.start()
    clock = setInterval(() => {
      nowMs = Date.now()
    }, 100)
  })

  onDestroy(() => {
    liveActivity.stop()
    if (clock) clearInterval(clock)
  })

  // ---------- derived dari store ----------
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

  const running = $derived(snapshot.sessions.filter((s) => s.status === 'running'))
  const waitingSessions = $derived(snapshot.sessions.filter((s) => s.status === 'waiting'))
  const doneSessions = $derived(snapshot.sessions.filter((s) => s.finished))
  const recentlyDone = $derived(doneSessions.slice(0, 3))

  const statusLabel = $derived(
    stale ? 'Disconnected' : active > 0 ? 'Running' : waiting > 0 ? 'Waiting' : 'Idle'
  )

  const headline = $derived(
    stale
      ? 'Gateway feed unavailable, retrying'
      : active > 0
        ? `${active} request${active === 1 ? '' : 's'} in flight${streams > 0 ? `, ${streams} streaming` : ''}`
        : recentlyDone.length > 0
          ? `Last request finished ${recentlyDone[0]?.status === 'ok' ? 'successfully' : 'with an error'}`
          : 'No active requests'
  )

  const lastRPS = $derived(history.length > 0 ? history.slice(-10) : [rps || 0])
  const peak = $derived(Math.max(...lastRPS, 0))

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

<div class="bg-surface border border-border rounded-lg overflow-hidden">
  <!-- ===== Header ===== -->
  <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-3">
    <div class="flex min-w-0 items-center gap-2.5">
      <span class="relative inline-flex h-2 w-2 shrink-0">
        <span class="h-2 w-2 rounded-full bg-accent" />
        {active > 0 && !stale && (
          <span
            class="absolute inset-0 h-2 w-2 rounded-full bg-accent/40"
            style={{ animation: 'live-ping 1.6s cubic-bezier(0, 0, 0.2, 1) infinite' }}
          />
        )}
      </span>
      <span class="text-[13px] font-semibold">Live Activity</span>
      <span class="rounded border border-accent/30 bg-accent-subtle px-1.5 py-px text-[10px] font-medium text-accent">
        {statusLabel}
      </span>
    </div>

    <div class="flex items-end gap-5">
      <div class="text-right">
        <div class="tnum text-[17px] font-semibold leading-none text-text">{rps.toFixed(1)}</div>
        <div class="mt-0.5 text-[10px] text-subtle">req/s</div>
      </div>
      <div class="text-right">
        <div class="tnum text-[17px] font-semibold leading-none text-text">{fmtRate(tps)}</div>
        <div class="mt-0.5 text-[10px] text-subtle">tok/s</div>
      </div>
    </div>
  </div>

  <!-- ===== Rolling throughput chart ===== -->
  <div class="px-4 pt-2">
    <div class="flex h-11 w-full items-end gap-px overflow-hidden rounded-t-md bg-surface/50">
      {#each lastRPS as val}
        <div
          class="w-full bg-accent/30 transition-[height] duration-300"
          style={{ height: `${Math.max(2, (val / Math.max(1, peak)) * 40)}px` }}
          title={`${val.toFixed(1)} req/s`}
        />
      {/each}
    </div>
    <div class="mt-0.5 flex justify-between text-[9px] text-subtle">
      <span>throughput, last {lastRPS.length} polls</span>
      <span class="tnum">{peak.toFixed(1)} req/s peak</span>
    </div>
  </div>

  <!-- ===== Aggregate chips ===== -->
  <div class="flex flex-wrap items-center gap-1.5 px-4 pt-2.5 pb-2">
    {active > 0 && <span class="tnum rounded border border-accent/25 bg-accent-subtle px-1.5 py-px text-[10px] text-accent">{active} active</span>}
    {inflight > 0 && <span class="tnum rounded border border-border bg-surface-2 px-1.5 py-px text-[10px] text-muted">{inflight} inflight</span>}
    {streams > 0 && <span class="tnum rounded border border-accent/25 bg-accent-subtle px-1.5 py-px text-[10px] text-accent">{streams} streaming</span>}
    {waiting > 0 && <span class="tnum rounded border border-warning/25 bg-warning-subtle px-1.5 py-px text-[10px] text-warning">{waiting} waiting</span>}
    {errors > 0 && <span class="tnum rounded border border-danger/25 bg-danger-subtle px-1.5 py-px text-[10px] text-danger">{errors} errors</span>}
  </div>

  <!-- ===== Pipeline indicator sweep ===== -->
  <div class="relative h-px w-full overflow-hidden bg-border/70">
    {active > 0 && !stale && (
      <div class="absolute inset-y-0 w-1/3 bg-accent" style={{ animation: 'live-sweep 1.6s cubic-bezier(0.4, 0, 0.2, 1) infinite' }} />
    )}
  </div>

  <!-- ===== Rows ===== -->
  {active > 0 || recentlyDone.length > 0 ? (
    <div class="divide-y divide-border/60">
      {#each [...running, ...waitingSessions].slice(0, 6) as s}
        <div class="flex items-center gap-3 px-4 py-2">
          <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5">
              <code class="truncate font-mono text-[11px] font-medium text-text">{s.model || 'unknown'}</code>
              {#if s.stream && (s.status === 'running' || s.status === 'waiting')}
                <span class="inline-block h-3 w-[2px] translate-y-[1px] rounded-full bg-accent" style={{ animation: 'live-cursor 1s steps(2, start) infinite' }} />
              {/if}
            </div>
            <div class="mt-1 flex items-center gap-2 text-[10px] text-subtle">
              <span>{s.finished ? (s.status === 'ok' ? 'completed' : 'failed') : s.step}</span>
              {#if s.provider}<span class="truncate">{s.provider}</span>{/if}
            </div>
          </div>
          <div class="tnum shrink-0 text-right text-[10px] text-muted">
            {fmtMs(s.finished ? s.elapsedMs : Math.max(s.elapsedMs, nowMs - (nowMs - s.elapsedMs)))}
          </div>
        </div>
      {/each}
      {active === 0 && recentlyDone.map((s) => (
        <div class="flex items-center gap-3 px-4 py-2 opacity-60">
          <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-success" />
          <div class="min-w-0 flex-1">
            <code class="truncate font-mono text-[11px] text-text">{s.model || 'unknown'}</code>
          </div>
          <div class="tnum shrink-0 text-right text-[10px] text-muted">{fmtMs(s.elapsedMs)}</div>
        </div>
      ))}
    </div>
  ) : (
    <div class="px-4 py-3 text-[11px] text-subtle">{headline}</div>
  )}

  {active > 6 && (
    <div class="border-t border-border/60 px-4 py-1.5 text-[10px] text-subtle">+{active - 6} more in flight</div>
  )}
</div>

<style>
  @keyframes live-ping {
    0% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.5; transform: scale(0.8); }
    100% { opacity: 1; transform: scale(1); }
  }

  @keyframes live-sweep {
    0% { left: 0; width: 0; }
    100% { left: 100%; width: 100%; }
  }

  @keyframes live-cursor {
    0%, 100% { opacity: 1; }
    50% { opacity: 0; }
  }
</style>