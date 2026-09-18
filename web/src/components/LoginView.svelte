<script lang="ts">
  import { api } from '../api/client'

  let {
    onSuccess,
  }: {
    onSuccess?: () => void
  } = $props()

  let password = $state('')
  let showPassword = $state(false)
  let isLoading = $state(false)
  let errorMessage = $state('')

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault()
    if (!password.trim() || isLoading) return

    isLoading = true
    errorMessage = ''

    try {
      const res = await api.login(password)
      if (res.success) {
        if (onSuccess) {
          onSuccess()
        } else {
          window.location.assign('/dashboard')
        }
      } else {
        errorMessage = res.error || 'Invalid password'
      }
    } catch (err: unknown) {
      errorMessage = err instanceof Error ? err.message : 'Invalid password'
    } finally {
      isLoading = false
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-bg p-4 relative overflow-hidden">
  <div class="landing-grid absolute inset-0 pointer-events-none" aria-hidden="true"></div>

  <div class="relative z-10 w-full max-w-md">
    <div class="text-center mb-8 flex flex-col items-center">
      <div class="size-14 rounded-2xl bg-surface border border-border-subtle shadow-[var(--shadow-warm)] flex items-center justify-center p-2.5 mb-4">
        <img src="/favicon.svg" alt="9router-go" class="w-full h-full object-contain" />
      </div>
      <h1 class="text-3xl font-bold text-primary mb-2">9router-go</h1>
      <p class="text-text-muted">Enter your password to access the dashboard</p>
    </div>

    <div class="bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-soft)] p-6">
      <form onsubmit={handleSubmit} class="flex flex-col gap-4">
        {#if errorMessage}
          <div class="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-500 text-xs">
            {errorMessage}
          </div>
        {/if}

        <div class="flex flex-col gap-2">
          <label for="login-password" class="text-sm font-medium text-text-main">Password</label>
          <div class="relative">
            <input
              id="login-password"
              type={showPassword ? 'text' : 'password'}
              placeholder="Enter password"
              bind:value={password}
              required
              class="w-full py-2.5 px-3 pr-10 text-sm text-text-main bg-surface-2 rounded-[10px] border border-transparent placeholder-text-muted/70 focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500 transition-colors"
            />
            <button
              type="button"
              onclick={() => (showPassword = !showPassword)}
              class="absolute inset-y-0 right-0 flex items-center pr-3 text-text-muted hover:text-text-main transition-colors cursor-pointer"
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            >
              <span class="material-symbols-outlined text-[20px]">
                {showPassword ? 'visibility_off' : 'visibility'}
              </span>
            </button>
          </div>
        </div>

        <button
          type="submit"
          disabled={isLoading || !password}
          class="w-full h-9 px-4 text-sm rounded-[10px] font-medium bg-brand-500 hover:bg-brand-600 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted disabled:cursor-not-allowed transition-colors flex items-center justify-center gap-2 cursor-pointer"
        >
          {#if isLoading}
            <span class="inline-block animate-spin rounded-full h-4 w-4 border-2 border-white/20 border-t-white"></span>
            <span>Logging in...</span>
          {:else}
            <span>Login</span>
          {/if}
        </button>

        <p class="text-xs text-center text-text-muted mt-2">
          Default password is <code class="bg-surface-2 px-1.5 py-0.5 rounded text-text-main font-mono">Mantep210</code>
        </p>
      </form>
    </div>
  </div>
</div>
