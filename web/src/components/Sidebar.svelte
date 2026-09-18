<script lang="ts">
  import { api } from '../api/client'
  import { TAB_ROUTES, type ActiveTab } from '../lib/router'

  export type { ActiveTab }

  let {
    activeTab = $bindable('endpoint'),
    navigate = (tab: ActiveTab) => {
      activeTab = tab
    },
    activeConnections = 0,
    totalConnections = 0,
    onClose,
  }: {
    activeTab: ActiveTab
    navigate?: (tab: ActiveTab, replace?: boolean) => void
    activeConnections?: number
    totalConnections?: number
    onClose?: () => void
  } = $props()

  let version = $state('')
  let isRemoteModalOpen = $state(false)

  // Media providers accordion (collapsed by default)
  let isMediaOpen = $state(false)

  $effect(() => {
    api
      .getSystemVersion()
      .then((v) => {
        version = (v as { currentVersion?: string }).currentVersion || ''
      })
      .catch(() => {})
  })


  function handleNav(tab: ActiveTab, e?: MouseEvent) {
    if (e) {
      if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey || e.button !== 0) return
      e.preventDefault()
    }
    navigate(tab)
    onClose?.()
  }

  const mainNavLinks = [
    { tab: 'endpoint' as ActiveTab, label: 'Endpoint & Key', icon: 'api' },
    { tab: 'connections' as ActiveTab, label: 'Providers', icon: 'dns' },
    { tab: 'combos' as ActiveTab, label: 'Combo & Vision Adapter', icon: 'layers' },
    { tab: 'analytics' as ActiveTab, label: 'Usage', icon: 'bar_chart' },
    { tab: 'quota' as ActiveTab, label: 'Quota Tracker', icon: 'data_usage' },
    { tab: 'token-saver' as ActiveTab, label: 'Token Saver', icon: 'savings' },
    { tab: 'cli-tools' as ActiveTab, label: 'CLI Tools', icon: 'terminal' },
  ] as const

  const mediaNavLinks = [
    { tab: 'media-embedding' as ActiveTab, label: 'Embedding', icon: 'data_array' },
    { tab: 'media-image' as ActiveTab, label: 'Text to Image', icon: 'brush' },
    { tab: 'media-tts' as ActiveTab, label: 'Text To Speech', icon: 'record_voice_over' },
    { tab: 'media-stt' as ActiveTab, label: 'Speech To Text', icon: 'mic' },
    { tab: 'media-video' as ActiveTab, label: 'Video', icon: 'movie' },
    { tab: 'media-web' as ActiveTab, label: 'Web Fetch & Search', icon: 'travel_explore' },
  ] as const

  const systemNavLinks = [
    { tab: 'proxy-pools' as ActiveTab, label: 'Proxy Pools', icon: 'lan' },
    { tab: 'skills' as ActiveTab, label: 'Skills', icon: 'extension' },
    { tab: 'console-log' as ActiveTab, label: 'Console Log', icon: 'terminal' },
  ] as const

  function isLinkActive(tab: ActiveTab): boolean {
    if (tab === 'endpoint') {
      return activeTab === 'endpoint' || activeTab === 'keys'
    }
    if (tab === 'console-log') {
      return activeTab === 'console-log' || activeTab === 'terminal'
    }
    return activeTab === tab
  }
</script>

<aside
  class="flex w-72 flex-col border-r border-border-subtle bg-sidebar backdrop-blur-xl transition-colors duration-300 min-h-full flex-shrink-0 select-none z-30"
>
  <!-- Window control / traffic lights -->
  <div class="flex items-center gap-2 px-6 pt-5 pb-2">
    <div class="w-3 h-3 rounded-full bg-[#FF5F56]"></div>
    <div class="w-3 h-3 rounded-full bg-[#FFBD2E]"></div>
    <div class="w-3 h-3 rounded-full bg-[#27C93F]"></div>
  </div>

  <!-- Brand header: 9router-go with official favicon.svg logo -->
  <div class="px-6 py-4 flex flex-col gap-2">
    <a
      href={TAB_ROUTES.endpoint}
      onclick={(e) => handleNav('endpoint', e)}
      class="flex items-center gap-3 cursor-pointer group"
    >
      <div
        class="flex items-center justify-center size-9 rounded-[10px] bg-surface-2 border border-border-subtle shadow-[var(--shadow-warm)] flex-shrink-0 group-hover:scale-105 transition-transform overflow-hidden p-1.5"
      >
        <img
          src="/favicon.svg"
          alt="9router-go"
          class="w-full h-full object-contain"
        />
      </div>
      <div class="flex flex-col min-w-0">
        <h1 class="text-lg font-semibold tracking-tight text-text-main truncate leading-snug">
          9router-go
        </h1>
        <span class="text-xs text-text-muted leading-tight">
          {version ? `v${version}` : 'v1.8.16'}
        </span>
      </div>
    </a>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 px-4 py-2 space-y-0.5 overflow-y-auto custom-scrollbar">
    <!-- 1-7 Main navigation links -->
    {#each mainNavLinks as item (item.tab)}
      {@const active = isLinkActive(item.tab)}
      <a
        href={TAB_ROUTES[item.tab]}
        onclick={(e) => handleNav(item.tab, e)}
        class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group cursor-pointer {active
          ? 'bg-primary/10 text-primary font-medium'
          : 'text-text-muted hover:bg-surface-2 hover:text-text-main'}"
      >
        <span
          class="material-symbols-outlined text-[18px] {active
            ? 'fill-1'
            : 'group-hover:text-primary transition-colors'}"
        >
          {item.icon}
        </span>
        <span class="text-[13px]">{item.label}</span>
      </a>
    {/each}

    <!-- System section header -->
    <div class="pt-3 mt-2 space-y-0.5">
      <p class="px-3 text-xs font-semibold text-text-muted/60 uppercase tracking-wider mb-2">
        System
      </p>

      <!-- 8. Media Providers accordion -->
      <button
        type="button"
        onclick={() => (isMediaOpen = !isMediaOpen)}
        class="w-full flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group cursor-pointer {activeTab.startsWith(
          'media-'
        )
          ? 'bg-primary/10 text-primary font-medium'
          : 'text-text-muted hover:bg-surface-2 hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[18px]">perm_media</span>
        <span class="text-[13px] flex-1 text-left">Media Providers</span>
        <span
          class="material-symbols-outlined text-[14px] transition-transform duration-200"
          style:transform={isMediaOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
        >
          expand_more
        </span>
      </button>

      {#if isMediaOpen}
        <div class="pl-4 space-y-0.5">
          {#each mediaNavLinks as item (item.tab)}
            {@const active = isLinkActive(item.tab)}
            <a
              href={TAB_ROUTES[item.tab]}
              onclick={(e) => handleNav(item.tab, e)}
              class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group cursor-pointer {active
                ? 'bg-primary/10 text-primary font-medium'
                : 'text-text-muted hover:bg-surface-2 hover:text-text-main'}"
            >
              <span
                class="material-symbols-outlined text-[16px] {active
                  ? 'fill-1'
                  : 'group-hover:text-primary transition-colors'}"
              >
                {item.icon}
              </span>
              <span class="text-[13px]">{item.label}</span>
            </a>
          {/each}
        </div>
      {/if}

      <!-- 9-11 System links: Proxy Pools, Skills, Console Log -->
      {#each systemNavLinks as item (item.tab)}
        {@const active = isLinkActive(item.tab)}
        <a
          href={TAB_ROUTES[item.tab]}
          onclick={(e) => handleNav(item.tab, e)}
          class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group cursor-pointer {active
            ? 'bg-primary/10 text-primary font-medium'
            : 'text-text-muted hover:bg-surface-2 hover:text-text-main'}"
        >
          <span
            class="material-symbols-outlined text-[18px] {active
              ? 'fill-1'
              : 'group-hover:text-primary transition-colors'}"
          >
            {item.icon}
          </span>
          <span class="text-[13px]">{item.label}</span>
        </a>
      {/each}

      <!-- 12. 9Remote (action button) -->
      <button
        type="button"
        onclick={() => (isRemoteModalOpen = true)}
        class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group w-full text-text-muted hover:bg-surface-2 hover:text-text-main cursor-pointer"
      >
        <span class="material-symbols-outlined text-[18px] group-hover:text-primary transition-colors">
          computer
        </span>
        <span class="text-[13px] font-medium">9Remote</span>
      </button>

      <!-- 13. 9English (external link) -->
      <a
        href="https://9english.net/"
        target="_blank"
        rel="noreferrer"
        class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group w-full text-text-muted hover:bg-surface-2 hover:text-text-main cursor-pointer"
      >
        <span class="material-symbols-outlined text-[18px] group-hover:text-primary transition-colors">
          translate
        </span>
        <span class="text-[13px] font-medium">9English</span>
      </a>

      <!-- 14. Settings -->
      <a
        href={TAB_ROUTES.settings}
        onclick={(e) => handleNav('settings', e)}
        class="flex items-center gap-3 px-3 py-1.5 rounded-lg transition-all group cursor-pointer {isLinkActive('settings')
          ? 'bg-primary/10 text-primary font-medium'
          : 'text-text-muted hover:bg-surface-2 hover:text-text-main'}"
      >
        <span
          class="material-symbols-outlined text-[18px] {isLinkActive('settings')
            ? 'fill-1'
            : 'group-hover:text-primary transition-colors'}"
        >
          settings
        </span>
        <span class="text-[13px]">Settings</span>
      </a>
    </div>
  </nav>

  <!-- Bottom connection summary -->
  <div class="p-4 border-t border-border-subtle">
    <div
      class="p-2.5 rounded-[10px] bg-surface border border-border-subtle flex items-center justify-between"
    >
      <div class="min-w-0">
        <p class="text-[11px] text-text-muted uppercase tracking-wide">Providers</p>
        <p class="text-xs font-semibold text-text-main mt-0.5 truncate">
          {activeConnections} <span class="text-text-muted font-normal">/ {totalConnections} active</span>
        </p>
      </div>
      <span class="material-symbols-outlined text-primary text-[18px]">radio_button_checked</span>
    </div>
  </div>
</aside>

<!-- 9Remote Modal -->
{#if isRemoteModalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div
      class="absolute inset-0 bg-black/50 backdrop-blur-sm"
      onclick={() => (isRemoteModalOpen = false)}
      onkeydown={(e) => e.key === 'Escape' && (isRemoteModalOpen = false)}
      role="button"
      tabindex="-1"
      aria-label="Close background"
    ></div>
    <div
      class="relative w-full max-w-sm bg-surface border border-border-subtle rounded-2xl shadow-2xl p-6 flex flex-col items-center text-center gap-4 z-10 animate-in fade-in zoom-in-95"
    >
      <div
        class="w-14 h-14 rounded-[14px] flex items-center justify-center bg-brand-500 shadow-[var(--shadow-warm)]"
      >
        <span class="material-symbols-outlined text-white text-[30px]">terminal</span>
      </div>
      <h2 class="text-lg font-bold text-text-main tracking-tight">9Remote</h2>
      <p class="text-xs text-text-muted leading-5">
        Access your terminal, desktop & files remotely from any browser.
      </p>
      <div class="flex items-center gap-2 w-full pt-2">
        <button
          type="button"
          onclick={() => (isRemoteModalOpen = false)}
          class="flex-1 py-2 px-3 text-sm rounded-lg border border-border text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer"
        >
          Close
        </button>
        <a
          href="https://github.com/decolua/9router"
          target="_blank"
          rel="noopener noreferrer"
          class="flex-1 py-2 px-3 text-sm rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-medium transition-colors cursor-pointer text-center"
        >
          Learn More
        </a>
      </div>
    </div>
  </div>
{/if}
