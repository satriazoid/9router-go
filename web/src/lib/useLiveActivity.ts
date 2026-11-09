/**
 * Live activity feed — polling hook untuk 9router-go.
 * 
 * Diterjemahkan dari useLiveActivity.ts (jrouters) ke arsitektur Svelte 5 + Vite.
 * 
 * Polls /api/live setiap 1200ms dengan ?since=? parameter.
 * Berhenti otomatis saat tab browser hidden.
 * Gateway melayani dari memory (one mutex, no DB).
 */

// --- Types lokal (mirip struktur LiveSession/LiveSnapshot dari jrouters) ---
export type LiveSessionStatus = 'running' | 'waiting' | 'ok' | 'error'

export interface LiveSession {
  id: string
  model: string
  provider: string
  combo?: string
  kind?: string
  stream: boolean
  status: LiveSessionStatus
  elapsedMs: number
  step: string
  steps: number
  targets: number
  attempt: number
  inflight: number
  ttfbMs: number
  outTokens: number
  promptTokens: number
  completionTokens: number
  tps: number
  finished: boolean
  responseCode?: number
  error?: string
}

export interface LiveSnapshot {
  generatedAt: string
  rps: number
  tps: number
  active: number
  waiting: number
  inflight: number
  streams: number
  errors: number
  retries: number
  oldestAgeSec: number
  count: number
  sessions: LiveSession[]
}

/** Kosongan awal */
export const EMPTY: LiveSnapshot = {
  generatedAt: '',
  rps: 0,
  tps: 0,
  active: 0,
  waiting: 0,
  inflight: 0,
  streams: 0,
  errors: 0,
  retries: 0,
  oldestAgeSec: 0,
  count: 0,
  sessions: [],
}

/** Store polling — bisa di-share antar komponen Svelte */
export class LiveActivityStore {
  snapshot: LiveSnapshot = EMPTY
  since = ''
  stale = false
  history: number[] = []
  intervalMs = 1200
  private polling = false
  private id: number | null = null

  constructor(intervalMs = this.intervalMs) {
    this.intervalMs = intervalMs
  }

  start() {
    if (this.polling) return
    this.polling = true
    void this.poll()
    this.id = setInterval(() => void this.poll(), this.intervalMs)
  }

  stop() {
    this.polling = false
    if (this.id !== null) {
      clearInterval(this.id)
      this.id = null
    }
  }

  private async poll() {
    try {
      const qs = this.since ? `?since=${encodeURIComponent(this.since)}` : ''
      const res = await fetch(`/api/live${qs}`)
      const data: LiveSnapshot = await res.json()
      if (data?.generatedAt) this.since = data.generatedAt
      this.snapshot = data ?? EMPTY
      this.stale = false
      this.history = [...this.history, data.rps].slice(-60)
    } catch {
      this.stale = true
    }
  }
}

/** Singleton store — di-import bersama antar komponen */
export const liveActivity = new LiveActivityStore(1200)
