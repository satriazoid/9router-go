/**
 * Live activity feed — polling store untuk 9router-go desktop widget.
 *
 * Memanggil GET /api/live setiap 1200ms dan menyimpan snapshot terbaru.
 * Berhenti saat widget di-unmount (onDestroy).
 */

export interface LiveSession {
  id: string
  model: string
  provider: string
  combo?: string
  kind?: string
  stream: boolean
  status: 'running' | 'waiting' | 'ok' | 'error'
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

/** Snapshot awal kosong */
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

/** Polling store — singleton, dipakai bersama antar komponen */
export class LiveActivityStore {
  snapshot: LiveSnapshot = EMPTY
  stale = false
  history: number[] = []
  intervalMs = 1200
  private polling = false
  private id: ReturnType<typeof setInterval> | null = null

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
      const res = await fetch(`/api/live`)
      const data = (await res.json()) as LiveSnapshot
      this.snapshot = data ?? EMPTY
      this.stale = false
      this.history = [...this.history, data.rps].slice(-60)
    } catch {
      this.stale = true
    }
  }
}

export const liveActivity = new LiveActivityStore()
