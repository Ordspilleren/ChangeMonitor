<script lang="ts">
  import type { Monitor } from '../types'

  interface Props {
    monitor: Monitor
    onsave: (m: Monitor) => void
    oncancel: () => void
  }

  let { monitor, onsave, oncancel }: Props = $props()

  let name = $state(monitor.name)
  let url = $state(monitor.url)
  let interval = $state(monitor.interval)
  let useChrome = $state(monitor.useChrome)
  let selectorType = $state(monitor.selector?.type ?? '')
  let selectorPaths = $state((monitor.selector?.paths ?? []).join('\n'))
  let filterContains = $state((monitor.filters?.contains ?? []).join('\n'))
  let filterNotContains = $state((monitor.filters?.notContains ?? []).join('\n'))
  let ignoreEmpty = $state(monitor.ignoreEmpty ?? false)

  let previewContent: string | null = $state(null)
  let previewError: string | null = $state(null)
  let previewing = $state(false)

  let valid = $derived(name.trim() !== '' && url.trim() !== '' && interval > 0)
  let canPreview = $derived(url.trim() !== '')

  function save(): void {
    if (!valid) return
    const paths = selectorPaths.split('\n').map((s) => s.trim()).filter(Boolean)
    const contains = filterContains.split('\n').map((s) => s.trim()).filter(Boolean)
    const notContains = filterNotContains.split('\n').map((s) => s.trim()).filter(Boolean)
    onsave({
      name: name.trim(),
      url: url.trim(),
      interval,
      useChrome,
      selector: selectorType ? { type: selectorType, paths } : undefined,
      filters: (contains.length || notContains.length) ? { contains, notContains } : undefined,
      ignoreEmpty,
    })
  }

  async function preview(): Promise<void> {
    previewContent = null
    previewError = null
    previewing = true
    try {
      const paths = selectorPaths.split('\n').map((s) => s.trim()).filter(Boolean)
      const res = await fetch('/api/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: url.trim(),
          useChrome,
          selector: selectorType ? { type: selectorType, paths } : undefined,
        }),
      })
      if (!res.ok) {
        previewError = await res.text()
      } else {
        const data = await res.json()
        previewContent = data.content
      }
    } catch (e) {
      previewError = String(e)
    } finally {
      previewing = false
    }
  }

  function trapFocus(node: HTMLElement): { destroy: () => void } {
    const focusable = (): HTMLElement[] =>
      Array.from(node.querySelectorAll<HTMLElement>(
        'button, input, select, textarea, [tabindex]:not([tabindex="-1"])'
      ))

    function handleKeydown(e: KeyboardEvent): void {
      if (e.key !== 'Tab') return
      const els = focusable()
      const first = els[0]
      const last = els[els.length - 1]
      if (e.shiftKey ? document.activeElement === first : document.activeElement === last) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus()
      }
    }

    node.addEventListener('keydown', handleKeydown)
    setTimeout(() => focusable()[0]?.focus(), 0)
    return { destroy: () => node.removeEventListener('keydown', handleKeydown) }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="modal-backdrop">
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title" use:trapFocus>
    <div class="modal-header">
      <h3 id="modal-title">{monitor.name ? 'Edit Monitor' : 'New Monitor'}</h3>
      <button class="close-btn" onclick={oncancel} aria-label="Close">×</button>
    </div>

    <div class="modal-body">
      <div class="form-group">
        <label for="m-name">Name</label>
        <input
          id="m-name"
          type="text"
          bind:value={name}
          placeholder="e.g. Product Page"
        />
      </div>

      <div class="form-group">
        <label for="m-url">URL</label>
        <input
          id="m-url"
          type="text"
          bind:value={url}
          placeholder="https://example.com/page"
        />
      </div>

      <div class="form-group">
        <label for="m-interval">Interval (minutes)</label>
        <input
          id="m-interval"
          type="number"
          bind:value={interval}
          min="1"
          step="1"
        />
        <span class="hint">How often to check this page for changes.</span>
      </div>

      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={useChrome} />
          Use Chrome for JS-rendered pages
        </label>
      </div>

      <div class="form-group">
        <label for="m-selector-type">Selector Type</label>
        <select id="m-selector-type" bind:value={selectorType}>
          <option value="">None (full page text)</option>
          <option value="css">CSS</option>
          <option value="json">JSON (gjson paths)</option>
        </select>
      </div>

      {#if selectorType}
        <div class="form-group">
          <label for="m-selector-paths">Selector Paths</label>
          <textarea
            id="m-selector-paths"
            bind:value={selectorPaths}
            rows="3"
            placeholder="One path per line"
          ></textarea>
          <span class="hint">
            {selectorType === 'css'
              ? 'CSS selectors, one per line. e.g. #price, .stock-status'
              : 'gjson paths, one per line. e.g. data.price, data.items.#.name'}
          </span>
        </div>
      {/if}

      <div class="form-group">
        <label for="m-contains">Contains filter</label>
        <textarea
          id="m-contains"
          bind:value={filterContains}
          rows="2"
          placeholder="One value per line — only notify when content contains this text"
        ></textarea>
      </div>

      <div class="form-group">
        <label for="m-not-contains">Does-not-contain filter</label>
        <textarea
          id="m-not-contains"
          bind:value={filterNotContains}
          rows="2"
          placeholder="One value per line — only notify when content does NOT contain this text"
        ></textarea>
      </div>

      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={ignoreEmpty} />
          Ignore empty content (skip notification if page returns nothing)
        </label>
      </div>

      {#if previewContent !== null || previewError !== null}
        <div class="form-group preview-result">
          <label>Preview</label>
          {#if previewError}
            <div class="preview-error">{previewError}</div>
          {:else}
            <pre class="preview-content">{previewContent}</pre>
          {/if}
        </div>
      {/if}
    </div>

    <div class="modal-footer">
      <button class="btn" onclick={oncancel}>Cancel</button>
      <button class="btn btn-secondary" onclick={preview} disabled={!canPreview || previewing}>
        {previewing ? 'Loading…' : 'Preview'}
      </button>
      <button class="btn btn-primary" onclick={save} disabled={!valid}>
        Save Monitor
      </button>
    </div>
  </div>
</div>
