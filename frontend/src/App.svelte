<script lang="ts">
  import { onMount } from 'svelte'
  import './app.css'
  import MonitorModal from './lib/MonitorModal.svelte'
  import type { Config, Monitor, MonitorTemplate, Notification } from './types'

  let config: Config | null = $state(null)
  let savedConfig: string | null = $state(null)
  let hasUnsavedChanges = $derived(
    config !== null && savedConfig !== null && JSON.stringify(config) !== savedConfig
  )
  let loading = $state(true)
  let saving = $state(false)
  let notification: Notification | null = $state(null)
  let notifTimer: ReturnType<typeof setTimeout> | null = null

  let showModal = $state(false)
  let editIndex = $state(-1)
  let editingMonitor: Monitor | null = $state(null)

  let editingAsTemplate = $state(false)
  let editTemplateIndex = $state(-1)

  onMount(async () => {
    try {
      const res = await fetch('/api/config')
      if (!res.ok) throw new Error(`Server returned ${res.status}`)
      config = await res.json() as Config
      config.monitors = config.monitors ?? []
      config.templates = config.templates ?? []
      if (!config.notifiers) config.notifiers = {}
      if (!config.notifiers.pushover) config.notifiers.pushover = { apiToken: '', userKey: '' }
      savedConfig = JSON.stringify(config)
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
      savedConfig = JSON.stringify(config)
      showNotif('success', 'Configuration saved.')
    } catch (e) {
      showNotif('error', 'Failed to save: ' + (e as Error).message)
    } finally {
      saving = false
    }
  }

  function openAdd(): void {
    editingMonitor = { name: '', url: '', useChrome: false, interval: 5, generic: { selector: { type: '', paths: [] } } }
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

  function onSaveAsTemplate(t: MonitorTemplate): void {
    if (!config) return
    config.templates = [...(config.templates ?? []), t]
  }

  function openFromTemplate(t: MonitorTemplate): void {
    editingMonitor = {
      name: '',
      url: '',
      useChrome: t.useChrome,
      interval: t.interval,
      httpHeaders: t.httpHeaders,
      generic: t.generic ? JSON.parse(JSON.stringify(t.generic)) : undefined,
      product: t.product ? JSON.parse(JSON.stringify(t.product)) : undefined,
      marketplace: t.marketplace ? JSON.parse(JSON.stringify(t.marketplace)) : undefined,
    }
    editIndex = -1
    showModal = true
  }

  function openEditTemplate(i: number): void {
    if (!config) return
    const tmpl = config.templates![i]
    editingMonitor = {
      name: tmpl.name,
      url: '',
      useChrome: tmpl.useChrome,
      interval: tmpl.interval,
      httpHeaders: tmpl.httpHeaders,
      generic: tmpl.generic ? JSON.parse(JSON.stringify(tmpl.generic)) : undefined,
      product: tmpl.product ? JSON.parse(JSON.stringify(tmpl.product)) : undefined,
      marketplace: tmpl.marketplace ? JSON.parse(JSON.stringify(tmpl.marketplace)) : undefined,
    }
    editTemplateIndex = i
    editingAsTemplate = true
    editIndex = -1
    showModal = true
  }

  function onTemplateEditSave(t: MonitorTemplate): void {
    if (!config) return
    config.templates![editTemplateIndex] = t
    config.templates = [...config.templates!]
    showModal = false
    editingAsTemplate = false
  }

  function deleteTemplate(i: number): void {
    if (!config) return
    config.templates = (config.templates ?? []).filter((_, idx) => idx !== i)
  }

  function featureLabel(t: MonitorTemplate): string {
    if (t.marketplace) return 'Marketplace'
    if (t.product) return 'Product'
    return 'Generic'
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
      {#if hasUnsavedChanges}
          <span class="unsaved-indicator">Unsaved changes</span>
        {/if}
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
                    {#if monitor.generic}
                      <span class="tag">Generic</span>
                    {/if}
                    {#if monitor.product}
                      <span class="tag tag-product">Product detection</span>
                    {/if}
                    {#if monitor.marketplace}
                      <span class="tag tag-site">Marketplace</span>
                    {/if}
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Templates -->
      <section class="card">
        <div class="section-header">
          <h2>Templates</h2>
        </div>

        {#if (config.templates ?? []).length === 0}
          <p class="empty">No templates yet — use <strong>Save as Template</strong> in the monitor editor.</p>
        {:else}
          <div class="queries">
            {#each (config.templates ?? []) as tmpl, i}
              <div class="query-card">
                <div class="query-header">
                  <div class="monitor-name-url">
                    <strong>{tmpl.name || 'Unnamed template'}</strong>
                  </div>
                  <div class="query-actions">
                    <button class="btn btn-sm btn-secondary" onclick={() => openFromTemplate(tmpl)}>Use</button>
                    <button class="btn btn-sm" onclick={() => openEditTemplate(i)}>Edit</button>
                    <button class="btn btn-sm btn-danger" onclick={() => deleteTemplate(i)}>Delete</button>
                  </div>
                </div>
                <div class="query-meta">
                  <div class="tags">
                    <span class="tag">Every {tmpl.interval}m</span>
                    {#if tmpl.useChrome}
                      <span class="tag tag-site">Chrome</span>
                    {/if}
                    <span class="tag">{featureLabel(tmpl)}</span>
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Pushover Notifications -->
      <section class="card">
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
    templateMode={editingAsTemplate}
    onsave={onModalSave}
    onsavetemplate={editingAsTemplate ? onTemplateEditSave : onSaveAsTemplate}
    oncancel={() => { showModal = false; editingAsTemplate = false }}
  />
{/if}
