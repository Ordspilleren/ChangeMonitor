<script lang="ts">
  import { onMount } from 'svelte'
  import './app.css'
  import MonitorModal from './lib/MonitorModal.svelte'
  import type { Config, Monitor, Notification } from './types'

  let config: Config | null = $state(null)
  let loading = $state(true)
  let saving = $state(false)
  let notification: Notification | null = $state(null)
  let notifTimer: ReturnType<typeof setTimeout> | null = null

  let showModal = $state(false)
  let editIndex = $state(-1)
  let editingMonitor: Monitor | null = $state(null)

  onMount(async () => {
    try {
      const res = await fetch('/api/config')
      if (!res.ok) throw new Error(`Server returned ${res.status}`)
      config = await res.json() as Config
      config.monitors = config.monitors ?? []
      if (!config.notifiers) config.notifiers = {}
      if (!config.notifiers.pushover) config.notifiers.pushover = { apiToken: '', userKey: '' }
    } catch (e) {
      showNotif('error', 'Failed to load configuration: ' + (e as Error).message)
    } finally {
      loading = false
    }
  })

  function showNotif(type: Notification['type'], text: string): void {
    notification = { type, text }
    if (notifTimer !== null) clearTimeout(notifTimer)
    notifTimer = setTimeout(() => (notification = null), 6000)
  }

  async function save(): Promise<void> {
    saving = true
    notification = null
    try {
      const res = await fetch('/api/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `Server returned ${res.status}`)
      }
      showNotif('success', 'Configuration saved. Monitor changes take effect on restart.')
    } catch (e) {
      showNotif('error', 'Failed to save: ' + (e as Error).message)
    } finally {
      saving = false
    }
  }

  function openAdd(): void {
    editingMonitor = { name: '', url: '', useChrome: false, interval: 5 }
    editIndex = -1
    showModal = true
  }

  function openEdit(i: number): void {
    if (!config) return
    editingMonitor = JSON.parse(JSON.stringify(config.monitors[i])) as Monitor
    editIndex = i
    showModal = true
  }

  function deleteMonitor(i: number): void {
    if (!config) return
    config.monitors = config.monitors.filter((_, idx) => idx !== i)
  }

  function onModalSave(m: Monitor): void {
    if (!config) return
    if (editIndex === -1) {
      config.monitors = [...config.monitors, m]
    } else {
      config.monitors[editIndex] = m
      config.monitors = [...config.monitors]
    }
    showModal = false
  }
</script>

<div class="app">
  <header>
    <div class="header-inner">
      <div class="logo">
        <svg width="26" height="26" viewBox="0 0 26 26" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
          <rect width="26" height="26" rx="6" fill="#3b82f6"/>
          <path d="M5 19 L9 9 L13 15 L17 9 L21 19" stroke="white" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        Change Monitor
      </div>
      <button
        class="btn btn-primary"
        onclick={save}
        disabled={saving || loading || !config}
      >
        {saving ? 'Saving…' : 'Save Changes'}
      </button>
    </div>
  </header>

  {#if notification}
    <div class="notification {notification.type}" role="alert">
      {notification.text}
    </div>
  {/if}

  {#if loading}
    <p class="loading">Loading configuration…</p>
  {:else if config}
    <main>
      <!-- Monitors -->
      <section class="card">
        <div class="section-header">
          <h2>Monitors</h2>
          <button class="btn btn-secondary" onclick={openAdd}>+ Add Monitor</button>
        </div>

        {#if config.monitors.length === 0}
          <p class="empty">No monitors yet — click <strong>Add Monitor</strong> to create one.</p>
        {:else}
          <div class="queries">
            {#each config.monitors as monitor, i}
              <div class="query-card">
                <div class="query-header">
                  <div class="monitor-name-url">
                    <strong>{monitor.name || 'Unnamed monitor'}</strong>
                    <a class="monitor-url" href="{monitor.url}" target="_blank" rel="noopener noreferrer">{monitor.url}</a>
                  </div>
                  <div class="query-actions">
                    <button class="btn btn-sm" onclick={() => openEdit(i)}>Edit</button>
                    <button class="btn btn-sm btn-danger" onclick={() => deleteMonitor(i)}>Delete</button>
                  </div>
                </div>
                <div class="query-meta">
                  <div class="tags">
                    <span class="tag">Every {monitor.interval}m</span>
                    {#if monitor.useChrome}
                      <span class="tag tag-site">Chrome</span>
                    {/if}
                    {#if monitor.selector?.type}
                      <span class="tag">{monitor.selector.type.toUpperCase()} selector</span>
                    {/if}
                    {#if monitor.ignoreEmpty}
                      <span class="tag">Ignore empty</span>
                    {/if}
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Pushover Notifications -->
      <section class="card">
        <h2>Pushover Notifications</h2>
        {#if config.notifiers.pushover}
        <div class="form-group">
          <label for="api-token">API Token</label>
          <input
            id="api-token"
            type="password"
            bind:value={config.notifiers.pushover.apiToken}
            placeholder="Your Pushover application token"
            autocomplete="off"
          />
        </div>
        <div class="form-group">
          <label for="user-key">User Key</label>
          <input
            id="user-key"
            type="password"
            bind:value={config.notifiers.pushover.userKey}
            placeholder="Your Pushover user key"
            autocomplete="off"
          />
        </div>
        {/if}
      </section>
    </main>
  {/if}
</div>

{#if showModal && editingMonitor}
  <MonitorModal
    monitor={editingMonitor}
    onsave={onModalSave}
    oncancel={() => (showModal = false)}
  />
{/if}
