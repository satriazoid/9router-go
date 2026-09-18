<script lang="ts">
  import { api, type ProviderConnection, type ProviderNode } from '../../api/client'
  import ProvidersOverviewGrid from './ProvidersOverviewGrid.svelte'
  import ProviderDetailView from './ProviderDetailView.svelte'
  import AddCompatibleNodeModal from './AddCompatibleNodeModal.svelte'

  interface Props {
    connections?: ProviderConnection[]
    providerNodes?: ProviderNode[]
    onRefresh: () => void
    selectedProviderId?: string | null
    onSelectProvider?: (id: string) => void
    onBackToOverview?: () => void
  }

  let {
    connections = [],
    providerNodes = [],
    onRefresh,
    selectedProviderId = $bindable(null),
    onSelectProvider,
    onBackToOverview
  }: Props = $props()

  let showAddOpenAIModal = $state(false)
  let showAddAnthropicModal = $state(false)
  let isSubmitting = $state(false)

  async function handleToggleAll(providerId: string, newActive: boolean) {
    const conns = connections.filter((c) => c.provider === providerId)
    await Promise.allSettled(
      conns.map((c) => api.updateConnection(c.id, { isActive: newActive ? 1 : 0 }))
    )
    onRefresh()
  }

  async function handleCreateNode(data: {
    name: string
    prefix: string
    baseUrl: string
    apiType?: 'chat' | 'responses'
    apiKey?: string
    type: string
  }) {
    isSubmitting = true
    try {
      const node = await api.createProviderNode({
        name: data.name,
        prefix: data.prefix,
        baseUrl: data.baseUrl,
        apiType: data.apiType,
        type: data.type as unknown as string
      })
      if (data.apiKey) {
        await api.createConnection({
          provider: node.id,
          authType: 'compatible',
          name: `${data.name} Key`,
          apiKey: data.apiKey
        })
      }
      showAddOpenAIModal = false
      showAddAnthropicModal = false
      onRefresh()
      selectedProviderId = node.id
      onSelectProvider?.(node.id)
    } catch (err) {
      alert(`Failed to add provider node: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      isSubmitting = false
    }
  }
</script>

{#if selectedProviderId}
  <ProviderDetailView
    providerId={selectedProviderId}
    {connections}
    {providerNodes}
    onBack={() => {
      selectedProviderId = null
      onBackToOverview?.()
    }}
    {onRefresh}
  />
{:else}
  <ProvidersOverviewGrid
    {connections}
    {providerNodes}
    onSelectProvider={(id) => {
      selectedProviderId = id
      onSelectProvider?.(id)
    }}
    onToggleAll={handleToggleAll}
    onAddAnthropic={() => (showAddAnthropicModal = true)}
    onAddOpenAI={() => (showAddOpenAIModal = true)}
  />
{/if}

<AddCompatibleNodeModal
  isOpen={showAddOpenAIModal}
  type="openai-compatible"
  {isSubmitting}
  onClose={() => (showAddOpenAIModal = false)}
  onSubmit={handleCreateNode}
/>
<AddCompatibleNodeModal
  isOpen={showAddAnthropicModal}
  type="anthropic-compatible"
  {isSubmitting}
  onClose={() => (showAddAnthropicModal = false)}
  onSubmit={handleCreateNode}
/>
