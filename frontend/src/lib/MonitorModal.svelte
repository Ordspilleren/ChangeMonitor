<script lang="ts">
  import type { Monitor, MonitorTemplate } from '../types'

  interface Props {
    monitor: Monitor
    templateMode?: boolean
    onsave: (m: Monitor) => void
    onsavetemplate?: (t: MonitorTemplate) => void
    oncancel: () => void
  }

  let { monitor, templateMode = false, onsave, onsavetemplate, oncancel }: Props = $props()

  let name = $state('')
  let url = $state('')
  let interval = $state(0)
  let useChrome = $state(false)
  let enabled = $state(true)
  let selectorType = $state('')
  let selectorPaths = $state('')
  let filterContains = $state('')
  let filterNotContains = $state('')
  let ignoreEmpty = $state(false)
  let featureType = $state<'generic' | 'product' | 'marketplace'>('generic')
  let trackStock = $state(false)
  let trackPrice = $state(false)
  let minPrice = $state<number | undefined>(undefined)
  let maxPrice = $state<number | undefined>(undefined)
  let mktSelector = $state('')
  let mktLinkSelector = $state('')
  let mktTitleSelector = $state('')
  let mktPriceSelector = $state('')
  let mktKeywords = $state('')
  let mktMaxPrice = $state<number | undefined>(undefined)
  let showAdvanced = $state(false)
  let httpHeaderEntries = $state<{ key: string; value: string }[]>([])

  $effect(() => {
    name = monitor.name
    url = monitor.url
    interval = monitor.interval
    useChrome = monitor.useChrome
    enabled = monitor.enabled ?? true
    selectorType = monitor.generic?.selector?.type ?? ''
    selectorPaths = (monitor.generic?.selector?.paths ?? []).join('\n')
    filterContains = (monitor.generic?.filters?.contains ?? []).join('\n')
    filterNotContains = (monitor.generic?.filters?.notContains ?? []).join('\n')
    ignoreEmpty = monitor.generic?.ignoreEmpty ?? false
    featureType = monitor.product !== undefined ? 'product' : monitor.marketplace !== undefined ? 'marketplace' : 'generic'
    trackStock = monitor.product?.trackStock ?? false
    trackPrice = monitor.product?.trackPrice ?? false
    minPrice = monitor.product?.minPrice
    maxPrice = monitor.product?.maxPrice
    mktSelector = monitor.marketplace?.selector ?? ''
    mktLinkSelector = monitor.marketplace?.linkSelector ?? ''
    mktTitleSelector = monitor.marketplace?.titleSelector ?? ''
    mktPriceSelector = monitor.marketplace?.priceSelector ?? ''
    mktKeywords = (monitor.marketplace?.keywords ?? []).join('\n')
    mktMaxPrice = monitor.marketplace?.maxPrice
    httpHeaderEntries = Object.entries(monitor.httpHeaders ?? {}).flatMap(([k, vals]) =>
      vals.map((v) => ({ key: k, value: v }))
    )
  })

  function addHeader(): void {
    httpHeaderEntries = [...httpHeaderEntries, { key: '', value: '' }]
  }

  function removeHeader(index: number): void {
    httpHeaderEntries = httpHeaderEntries.filter((_, i) => i !== index)
  }

  let previewContent: string | null = $state(null)
  let previewError: string | null = $state(null)
  let previewing = $state(false)

  let valid = $derived(
    name.trim() !== '' && (templateMode || url.trim() !== '') && interval > 0 &&
    (featureType !== 'product' || trackStock || trackPrice) &&
    (featureType !== 'marketplace' || mktSelector.trim() !== '')
  )
  let canPreview = $derived(
    url.trim() !== '' &&
    (featureType !== 'product' || trackStock || trackPrice) &&
    (featureType !== 'marketplace' || mktSelector.trim() !== '')
  )

  function save(): void {
    if (!valid) return
    const paths = selectorPaths.split('\n').map((s) => s.trim()).filter(Boolean)
    const contains = filterContains.split('\n').map((s) => s.trim()).filter(Boolean)
    const notContains = filterNotContains.split('\n').map((s) => s.trim()).filter(Boolean)
    const httpHeaders: Record<string, string[]> = {}
    for (const { key, value } of httpHeaderEntries) {
      const k = key.trim()
      if (!k) continue
      if (!httpHeaders[k]) httpHeaders[k] = []
      httpHeaders[k].push(value)
    }
    let generic = undefined
    let product = undefined
    let marketplace = undefined
    if (featureType === 'product') {
      product = {
        trackStock,
        trackPrice,
        minPrice: trackPrice && minPrice !== undefined ? minPrice : undefined,
        maxPrice: trackPrice && maxPrice !== undefined ? maxPrice : undefined,
      }
    } else if (featureType === 'marketplace') {
      marketplace = {
        selector: mktSelector.trim(),
        linkSelector: mktLinkSelector.trim() || undefined,
        titleSelector: mktTitleSelector.trim() || undefined,
        priceSelector: mktPriceSelector.trim() || undefined,
        keywords: mktKeywords.split('\n').map((s) => s.trim()).filter(Boolean),
        maxPrice: mktMaxPrice,
      }
    } else {
      generic = {
        selector: { type: selectorType, paths },
        filters: (contains.length || notContains.length) ? { contains, notContains } : undefined,
        ignoreEmpty,
      }
    }
    onsave({
      name: name.trim(),
      url: url.trim(),
      interval,
      useChrome,
      enabled,
      httpHeaders: Object.keys(httpHeaders).length ? httpHeaders : undefined,
      generic,
      product,
      marketplace,
    })
  }

  function saveAsTemplate(): void {
    if (!onsavetemplate) return
    const paths = selectorPaths.split('\n').map((s) => s.trim()).filter(Boolean)
    const contains = filterContains.split('\n').map((s) => s.trim()).filter(Boolean)
    const notContains = filterNotContains.split('\n').map((s) => s.trim()).filter(Boolean)
    const httpHeaders: Record<string, string[]> = {}
    for (const { key, value } of httpHeaderEntries) {
      const k = key.trim()
      if (!k) continue
      if (!httpHeaders[k]) httpHeaders[k] = []
      httpHeaders[k].push(value)
    }
    let generic = undefined
    let product = undefined
    let marketplace = undefined
    if (featureType === 'product') {
      product = {
        trackStock,
        trackPrice,
        minPrice: trackPrice && minPrice !== undefined ? minPrice : undefined,
        maxPrice: trackPrice && maxPrice !== undefined ? maxPrice : undefined,
      }
    } else if (featureType === 'marketplace') {
      marketplace = {
        selector: mktSelector.trim(),
        linkSelector: mktLinkSelector.trim() || undefined,
        titleSelector: mktTitleSelector.trim() || undefined,
        priceSelector: mktPriceSelector.trim() || undefined,
        keywords: mktKeywords.split('\n').map((s) => s.trim()).filter(Boolean),
        maxPrice: mktMaxPrice,
      }
    } else {
      generic = {
        selector: { type: selectorType, paths },
        filters: (contains.length || notContains.length) ? { contains, notContains } : undefined,
        ignoreEmpty,
      }
    }
    const templateName = templateMode ? (name.trim() || 'New template') : (name.trim() ? `${name.trim()} template` : 'New template')
    onsavetemplate({
      name: templateName,
      interval,
      useChrome,
      httpHeaders: Object.keys(httpHeaders).length ? httpHeaders : undefined,
      generic,
      product,
      marketplace,
    })
  }

  async function preview(): Promise<void> {
    previewContent = null
    previewError = null
    previewing = true
    try {
      const paths = selectorPaths.split('\n').map((s) => s.trim()).filter(Boolean)
      const httpHeaders: Record<string, string[]> = {}
      for (const { key, value } of httpHeaderEntries) {
        const k = key.trim()
        if (!k) continue
        if (!httpHeaders[k]) httpHeaders[k] = []
        httpHeaders[k].push(value)
      }
      let generic = undefined
      let product = undefined
      let marketplace = undefined
      if (featureType === 'product') {
        product = {
          trackStock,
          trackPrice,
          minPrice,
          maxPrice,
        }
      } else if (featureType === 'marketplace') {
        marketplace = {
          selector: mktSelector.trim(),
          linkSelector: mktLinkSelector.trim() || undefined,
          titleSelector: mktTitleSelector.trim() || undefined,
          priceSelector: mktPriceSelector.trim() || undefined,
          keywords: mktKeywords.split('\n').map((s) => s.trim()).filter(Boolean),
          maxPrice: mktMaxPrice,
        }
      } else {
        const contains = filterContains.split('\n').map((s) => s.trim()).filter(Boolean)
        const notContains = filterNotContains.split('\n').map((s) => s.trim()).filter(Boolean)
        generic = {
          selector: { type: selectorType, paths },
          filters: (contains.length || notContains.length) ? { contains, notContains } : undefined,
          ignoreEmpty,
        }
      }
      const body: Record<string, unknown> = {
        name: name.trim() || 'Preview',
        url: url.trim(),
        interval: interval > 0 ? interval : 1,
        useChrome,
        generic,
        product,
        marketplace,
      }
      if (Object.keys(httpHeaders).length) body.httpHeaders = httpHeaders
      const res = await fetch('/api/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        previewError = await res.text()
      } else {
        const data = await res.json()
        previewContent = data.content ?? ''
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
      <h3 id="modal-title">{templateMode ? 'Edit Template' : (monitor.name ? 'Edit Monitor' : 'New Monitor')}</h3>
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
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={enabled} />
          Enabled
        </label>
        <span class="hint">When disabled, the monitor will not run and no notifications will be sent.</span>
      </div>

      <div class="form-group">
        <label for="m-feature-type">Detection Feature</label>
        <select id="m-feature-type" bind:value={featureType}>
          <option value="generic">Generic (page content)</option>
          <option value="product">Product (stock &amp; price)</option>
          <option value="marketplace">Marketplace</option>
        </select>
        <span class="hint">
          {featureType === 'product'
            ? 'Automatically extract stock status and price from single-product pages.'
            : featureType === 'marketplace'
            ? 'Monitor an online marketplace for new listings matching keywords.'
            : 'Monitor page content for changes using selectors and filters.'}
        </span>
      </div>

      {#if featureType === 'marketplace'}
        <div class="form-group">
          <label for="m-mkt-selector">Listing selector <span aria-hidden="true">*</span></label>
          <input
            id="m-mkt-selector"
            type="text"
            bind:value={mktSelector}
            placeholder="e.g. article.listing-card"
          />
          <span class="hint">CSS selector that matches one element per listing on the page.</span>
        </div>

        <div class="form-group">
          <label for="m-mkt-link-selector">Link selector (optional)</label>
          <input
            id="m-mkt-link-selector"
            type="text"
            bind:value={mktLinkSelector}
            placeholder="e.g. a.listing-link"
          />
          <span class="hint">CSS selector for the &lt;a&gt; element inside each listing. Leave blank to use the first link found.</span>
        </div>

        <div class="form-group">
          <label for="m-mkt-title-selector">Title selector (optional)</label>
          <input
            id="m-mkt-title-selector"
            type="text"
            bind:value={mktTitleSelector}
            placeholder="e.g. h2.title"
          />
          <span class="hint">CSS selector for the title element. Leave blank to use the first heading or full text.</span>
        </div>

        <div class="form-group">
          <label for="m-mkt-price-selector">Price selector (optional)</label>
          <input
            id="m-mkt-price-selector"
            type="text"
            bind:value={mktPriceSelector}
            placeholder="e.g. span.price"
          />
          <span class="hint">CSS selector for the price element. Leave blank to auto-detect a price-like value.</span>
        </div>

        <div class="form-group">
          <label for="m-mkt-keywords">Keywords (optional)</label>
          <textarea
            id="m-mkt-keywords"
            bind:value={mktKeywords}
            rows="3"
            placeholder="One keyword or phrase per line e.g.&#10;macbook pro&#10;iphone 14"
          ></textarea>
          <span class="hint">Only notify for listings whose title matches at least one keyword.</span>
        </div>

        <div class="form-group">
          <label for="m-mkt-max-price">Max price (optional)</label>
          <input
            id="m-mkt-max-price"
            type="number"
            bind:value={mktMaxPrice}
            min="0"
            step="1"
            placeholder="No maximum"
          />
          <span class="hint">Only notify for listings at or below this price. Leave blank to notify for all listings.</span>
        </div>
      {/if}

      {#if featureType === 'generic'}
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
      {/if}

      {#if featureType === 'product'}
        <div class="form-group product-detection-section">
          <label class="checkbox-label">
            <input type="checkbox" bind:checked={trackStock} />
            Notify when stock status changes
          </label>
          <label class="checkbox-label">
            <input type="checkbox" bind:checked={trackPrice} />
            Notify when price changes
          </label>
          {#if trackPrice}
            <div class="price-thresholds">
              <div class="price-threshold-row">
                <label for="m-min-price">Min price (notify if price ≥)</label>
                <input
                  id="m-min-price"
                  type="number"
                  bind:value={minPrice}
                  min="0"
                  step="0.01"
                  placeholder="No minimum"
                />
              </div>
              <div class="price-threshold-row">
                <label for="m-max-price">Max price (notify if price ≤)</label>
                <input
                  id="m-max-price"
                  type="number"
                  bind:value={maxPrice}
                  min="0"
                  step="0.01"
                  placeholder="No maximum"
                />
              </div>
              <span class="hint">Leave blank to notify on any price change. Set max price to get alerts when an item drops below a target price.</span>
            </div>
          {/if}
        </div>
      {/if}

      <div class="form-group">
        <button
          type="button"
          class="btn btn-toggle-advanced"
          onclick={() => (showAdvanced = !showAdvanced)}
          aria-expanded={showAdvanced}
        >
          {showAdvanced ? '▾' : '▸'} Advanced
        </button>
      </div>

      {#if showAdvanced}
        <div class="form-group advanced-section">
          <label>HTTP Headers</label>
          {#each httpHeaderEntries as entry, i}
            <div class="header-row">
              <input
                type="text"
                bind:value={entry.key}
                placeholder="Header name"
                aria-label="Header name"
              />
              <input
                type="text"
                bind:value={entry.value}
                placeholder="Value"
                aria-label="Header value"
              />
              <button
                type="button"
                class="btn btn-remove-header"
                onclick={() => removeHeader(i)}
                aria-label="Remove header"
              >×</button>
            </div>
          {/each}
          <button type="button" class="btn btn-add-header" onclick={addHeader}>
            + Add Header
          </button>
        </div>
      {/if}

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
      {#if onsavetemplate && !templateMode}
        <button class="btn btn-secondary" onclick={saveAsTemplate}>
          Save as Template
        </button>
      {/if}
      {#if templateMode}
        <button class="btn btn-primary" onclick={saveAsTemplate} disabled={!valid}>
          Save Template
        </button>
      {:else}
        <button class="btn btn-primary" onclick={save} disabled={!valid}>
          Save Monitor
        </button>
      {/if}
    </div>
  </div>
</div>
