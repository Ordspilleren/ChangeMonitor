package monitor

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/tidwall/gjson"
)

// MonitorClient retrieves content from a URL.
type MonitorClient interface {
	GetContent(url string, headers http.Header) (io.ReadCloser, error)
}

// Storage persists and retrieves recorded content for each monitor.
type Storage interface {
	GetContent(id string) string
	WriteContent(id string, content string)
	// Cleanup removes persisted state for any ID not present in activeIDs.
	Cleanup(activeIDs []string) error
}

// NotifierService dispatches change notifications.
type NotifierService interface {
	Notify(ctx context.Context, subject, message string) error
}

// MonitorService manages a collection of monitors.
type MonitorService struct {
	wg           sync.WaitGroup
	monitors     Monitors
	httpClient   *HTTPClient
	chromeClient *ChromeClient
	storage      Storage
	notifier     NotifierService
	chromePath   string
	chromeWsURL  string
}

// HTTPClient fetches page content over plain HTTP.
type HTTPClient struct {
	client http.Client
}

// ChromeClient fetches page content using a headless Chrome browser via chromedp.
type ChromeClient struct {
	allocCtx    context.Context
	cancelAlloc context.CancelFunc
}

// ProductDetection configures automatic stock and price change detection for
// single-product pages. When enabled, the normal content-hash check is
// replaced by a structured product-state comparison.
type ProductDetection struct {
	Enabled    bool     `json:"enabled"`
	TrackStock bool     `json:"trackStock,omitempty"`
	TrackPrice bool     `json:"trackPrice,omitempty"`
	MinPrice   *float64 `json:"minPrice,omitempty"`
	MaxPrice   *float64 `json:"maxPrice,omitempty"`
}

// ProductState holds the last-observed price and availability for a product monitor.
type ProductState struct {
	InStock bool    `json:"inStock"`
	Price   float64 `json:"price"`
}

// Monitor describes a single URL to be watched for changes.
type Monitor struct {
	Name             string            `json:"name"`
	URL              string            `json:"url"`
	HTTPHeaders      http.Header       `json:"httpHeaders,omitempty"`
	UseChrome        bool              `json:"useChrome"`
	Interval         time.Duration     `json:"interval"`
	Selector         Selector          `json:"selector,omitempty"`
	Filters          *Filters          `json:"filters,omitempty"`
	IgnoreEmpty      bool              `json:"ignoreEmpty,omitempty"`
	ProductDetection *ProductDetection `json:"productDetection,omitempty"`

	notifier NotifierService
	storage  Storage
	client   MonitorClient
	id       string
	started  bool
	ticker   *time.Ticker
	done     chan struct{}
}

// Monitors is a slice of Monitor values.
type Monitors []Monitor

// Selector describes how to extract the relevant content from a fetched document.
type Selector struct {
	Type  string   `json:"type,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

// Filters defines content-based conditions that must match before a notification
// is sent.
type Filters struct {
	Contains    []string `json:"contains,omitempty"`
	NotContains []string `json:"notContains,omitempty"`
}

// NewMonitorService creates a MonitorService with a plain HTTP client ready to
// use. Call SetupChrome before Start if any monitor has UseChrome set.
func NewMonitorService(monitors Monitors, storage Storage, notifier NotifierService) *MonitorService {
	return &MonitorService{
		monitors:   monitors,
		storage:    storage,
		notifier:   notifier,
		httpClient: &HTTPClient{client: http.Client{}},
	}
}

// SetupChrome configures the headless-browser client. If wsURL is non-empty it
// connects to an external browser at that DevTools WebSocket address; otherwise
// it launches a local Chrome binary at chromePath. Chrome is only started when
// at least one monitor has UseChrome set to true.
func (ms *MonitorService) SetupChrome(chromePath, wsURL string) error {
	ms.chromePath = chromePath
	ms.chromeWsURL = wsURL
	for _, m := range ms.monitors {
		if !m.UseChrome {
			continue
		}
		var (
			client *ChromeClient
			err    error
		)
		if wsURL != "" {
			client, err = newRemoteChromeClient(wsURL)
		} else {
			client, err = newLocalChromeClient(chromePath)
		}
		if err != nil {
			return fmt.Errorf("monitor: setup chrome: %w", err)
		}
		ms.chromeClient = client
		return nil
	}
	return nil
}

// AddMonitors appends additional monitors to the service.
func (ms *MonitorService) AddMonitors(monitors ...Monitor) {
	ms.monitors = append(ms.monitors, monitors...)
}

// Reload stops all running monitors, replaces them with the provided list, and
// starts them again. The Chrome client is kept alive across reloads; it is only
// initialized here if it has not been set up yet.
func (ms *MonitorService) Reload(monitors Monitors) error {
	for i := range ms.monitors {
		if ms.monitors[i].started {
			ms.monitors[i].Stop()
		}
	}
	ms.wg.Wait()

	ms.monitors = monitors
	if ms.chromeClient == nil {
		if err := ms.SetupChrome(ms.chromePath, ms.chromeWsURL); err != nil {
			return err
		}
	}
	ms.Start()
	return nil
}

// Start initializes every monitor and begins polling. It returns immediately;
// monitors run in background goroutines. It also cleans up state files for any
// monitors that are no longer present (e.g. removed manually from the config).
func (ms *MonitorService) Start() {
	activeIDs := make([]string, 0, len(ms.monitors)*2)
	for i, m := range ms.monitors {
		id := generateSHA1(m.Name)
		activeIDs = append(activeIDs, id)
		if m.ProductDetection != nil && m.ProductDetection.Enabled {
			activeIDs = append(activeIDs, id+"_product")
		}
		ms.monitors[i].init(ms)
		if err := ms.monitors[i].start(&ms.wg); err != nil {
			log.Printf("monitor: failed to start %q: %v", ms.monitors[i].Name, err)
		}
	}
	if err := ms.storage.Cleanup(activeIDs); err != nil {
		log.Printf("monitor: cleanup storage: %v", err)
	}
}

// Shutdown stops all running monitors, waits for them to finish, then closes
// the Chrome browser if one was started.
func (ms *MonitorService) Shutdown() {
	for i := range ms.monitors {
		if ms.monitors[i].started {
			ms.monitors[i].Stop()
		}
	}
	ms.wg.Wait()
	if ms.chromeClient != nil {
		ms.chromeClient.close()
	}
}

// PreviewRequest holds the parameters needed to fetch and process content for a
// preview without persisting any state.
type PreviewRequest struct {
	URL              string            `json:"url"`
	HTTPHeaders      http.Header       `json:"httpHeaders,omitempty"`
	UseChrome        bool              `json:"useChrome"`
	Selector         Selector          `json:"selector"`
	ProductDetection *ProductDetection `json:"productDetection,omitempty"`
}

// PreviewResult holds the outcome of a preview request. Exactly one of Content
// or ProductState will be populated depending on whether product detection is
// enabled.
type PreviewResult struct {
	Content      string        `json:"content,omitempty"`
	ProductState *ProductState `json:"productState,omitempty"`
}

// Preview fetches and processes content for req without recording anything.
func (ms *MonitorService) Preview(req PreviewRequest) (PreviewResult, error) {
	var client MonitorClient
	if req.UseChrome {
		if ms.chromeClient == nil {
			return PreviewResult{}, fmt.Errorf("chrome client not initialised")
		}
		client = ms.chromeClient
	} else {
		client = ms.httpClient
	}

	content, err := client.GetContent(req.URL, req.HTTPHeaders)
	if err != nil {
		return PreviewResult{}, err
	}
	defer content.Close()

	if req.ProductDetection != nil && req.ProductDetection.Enabled {
		body, err := io.ReadAll(content)
		if err != nil {
			return PreviewResult{}, fmt.Errorf("preview: read body: %w", err)
		}
		ps, err := extractProductData(body)
		if err != nil {
			return PreviewResult{}, err
		}
		return PreviewResult{ProductState: ps}, nil
	}

	text, err := processContent(content, req.Selector)
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{Content: text}, nil
}

// NewMonitor is a convenience constructor for a basic monitor.
func NewMonitor(name, url string, intervalMinutes int64) *Monitor {
	return &Monitor{
		Name:     name,
		URL:      url,
		Interval: time.Duration(intervalMinutes),
	}
}

// AddCSSSelectors sets the monitor to extract content via the given CSS selectors.
func (m *Monitor) AddCSSSelectors(selectors ...string) {
	m.Selector.Type = "css"
	m.Selector.Paths = selectors
}

// AddJSONSelectors sets the monitor to extract content via the given gjson paths.
func (m *Monitor) AddJSONSelectors(selectors ...string) {
	m.Selector.Type = "json"
	m.Selector.Paths = selectors
}

// IsRunning reports whether the monitor's polling loop is active.
func (m *Monitor) IsRunning() bool {
	return m.started
}

// Stop signals the monitor to stop its polling loop.
func (m *Monitor) Stop() {
	m.done <- struct{}{}
}

func (m *Monitor) init(ms *MonitorService) {
	m.id = generateSHA1(m.Name)
	m.done = make(chan struct{}, 1)
	m.ticker = time.NewTicker(m.Interval * time.Minute)
	m.storage = ms.storage
	m.notifier = ms.notifier
	if m.UseChrome {
		m.client = ms.chromeClient
	} else {
		m.client = ms.httpClient
	}
}

func (m *Monitor) start(wg *sync.WaitGroup) error {
	if m.started {
		return errors.New("monitor is already started")
	}
	wg.Add(1)
	m.started = true
	go func() {
		defer func() {
			wg.Done()
			m.ticker.Stop()
			m.started = false
		}()
		m.check()
		for {
			select {
			case <-m.done:
				return
			case <-m.ticker.C:
				m.check()
			}
		}
	}()
	return nil
}

func generateSHA1(input string) string {
	h := sha1.New()
	h.Write([]byte(input))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (m *Monitor) check() {
	log.Printf("monitor: checking %s", m.URL)

	content, err := m.client.GetContent(m.URL, m.HTTPHeaders)
	if err != nil {
		log.Printf("monitor: get content: %v", err)
		return
	}
	defer content.Close()

	if m.ProductDetection != nil && m.ProductDetection.Enabled {
		m.checkProduct(content)
		return
	}

	processed, err := processContent(content, m.Selector)
	if err != nil {
		log.Printf("monitor: process content: %v", err)
		return
	}

	if m.IgnoreEmpty && processed == "" {
		log.Print("monitor: content is empty, ignoring")
		return
	}

	if m.Filters != nil && !filterMatch(*m.Filters, processed) {
		log.Print("monitor: no filter matched, ignoring")
		return
	}

	stored := m.storage.GetContent(m.id)
	if stored == processed {
		log.Printf("monitor: no change detected, next check in %s", m.Interval*time.Minute)
		return
	}

	m.storage.WriteContent(m.id, processed)
	log.Printf("monitor: %q has changed", m.Name)
	if err := m.notifier.Notify(
		context.Background(),
		fmt.Sprintf("ChangeMonitor: %s has changed!", m.Name),
		fmt.Sprintf("%s changed.\n\n---\n(changed) %.200s\n\n(into) %.200s\n---", m.URL, stored, processed),
	); err != nil {
		log.Printf("monitor: notify: %v", err)
	}
}

func (m *Monitor) checkProduct(content io.ReadCloser) {
	pd := m.ProductDetection
	if !pd.TrackStock && !pd.TrackPrice {
		log.Printf("monitor: product detection enabled but neither stock nor price tracking is configured")
		return
	}

	body, err := io.ReadAll(content)
	if err != nil {
		log.Printf("monitor: product detection: read body: %v", err)
		return
	}

	current, err := extractProductData(body)
	if err != nil {
		log.Printf("monitor: product detection: extract: %v", err)
		return
	}
	if current == nil {
		log.Printf("monitor: product detection: no product data found on page %s", m.URL)
		return
	}

	// Load and persist product state.
	storedJSON := m.storage.GetContent(m.id + "_product")
	var stored *ProductState
	if storedJSON != "" {
		stored = &ProductState{}
		if err := json.Unmarshal([]byte(storedJSON), stored); err != nil {
			log.Printf("monitor: product detection: parse stored state: %v", err)
			stored = nil
		}
	}
	stateJSON, _ := json.Marshal(current)
	m.storage.WriteContent(m.id+"_product", string(stateJSON))

	if stored == nil {
		log.Printf("monitor: initial product state recorded for %q (inStock=%v price=%.2f)", m.Name, current.InStock, current.Price)
		return
	}

	var changes []string

	if pd.TrackStock && current.InStock != stored.InStock {
		if current.InStock {
			changes = append(changes, "back in stock")
		} else {
			changes = append(changes, "now out of stock")
		}
	}

	if pd.TrackPrice && current.Price != stored.Price {
		meetsMin := pd.MinPrice == nil || current.Price >= *pd.MinPrice
		meetsMax := pd.MaxPrice == nil || current.Price <= *pd.MaxPrice
		if meetsMin && meetsMax {
			changes = append(changes, fmt.Sprintf("price changed from %.2f to %.2f", stored.Price, current.Price))
		}
	}

	if len(changes) == 0 {
		log.Printf("monitor: no relevant product change for %q, next check in %s", m.Name, m.Interval*time.Minute)
		return
	}

	changeStr := strings.Join(changes, "; ")
	log.Printf("monitor: %q product change: %s", m.Name, changeStr)
	if err := m.notifier.Notify(
		context.Background(),
		fmt.Sprintf("ChangeMonitor: %s – %s", m.Name, changeStr),
		fmt.Sprintf("%s\n\n%s\n\nURL: %s", m.Name, changeStr, m.URL),
	); err != nil {
		log.Printf("monitor: notify: %v", err)
	}
}

// extractProductData scans raw HTML for structured product information.
// It first tries schema.org JSON-LD, then Open Graph / product meta tags.
// Returns nil (no error) when no product data is found on the page.
func extractProductData(body []byte) (*ProductState, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("extract product: parse html: %w", err)
	}

	// --- JSON-LD ---
	var state *ProductState
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return true
		}
		var top any
		if err := json.Unmarshal([]byte(raw), &top); err != nil {
			return true
		}
		// @graph wrapper
		if m, ok := top.(map[string]any); ok {
			if graph, ok := m["@graph"].([]any); ok {
				for _, item := range graph {
					if ps := productStateFromLDNode(item); ps != nil {
						state = ps
						return false
					}
				}
				return true
			}
		}
		// Array of nodes
		if arr, ok := top.([]any); ok {
			for _, item := range arr {
				if ps := productStateFromLDNode(item); ps != nil {
					state = ps
					return false
				}
			}
			return true
		}
		// Single node
		if ps := productStateFromLDNode(top); ps != nil {
			state = ps
			return false
		}
		return true
	})
	if state != nil {
		return state, nil
	}

	// --- Open Graph / product meta tags ---
	var price float64
	var hasPrice, hasStock, inStock bool
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		prop := s.AttrOr("property", "")
		content := s.AttrOr("content", "")
		switch prop {
		case "product:price:amount", "og:price:amount":
			if p, ok := parsePrice(content); ok {
				price = p
				hasPrice = true
			}
		case "product:availability", "og:availability":
			hasStock = true
			inStock = isInStockString(content)
		}
	})
	if hasPrice || hasStock {
		return &ProductState{InStock: inStock, Price: price}, nil
	}

	return nil, nil
}

func productStateFromLDNode(node any) *ProductState {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	typeVal, _ := m["@type"].(string)
	switch typeVal {
	case "Product":
		offersRaw, ok := m["offers"]
		if !ok {
			return nil
		}
		return productStateFromOffers(offersRaw)
	case "Offer", "AggregateOffer":
		return productStateFromOffer(m)
	}
	return nil
}

func productStateFromOffers(offers any) *ProductState {
	switch v := offers.(type) {
	case map[string]any:
		return productStateFromOffer(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if ps := productStateFromOffer(m); ps != nil {
					return ps
				}
			}
		}
	}
	return nil
}

func productStateFromOffer(offer map[string]any) *ProductState {
	var state ProductState
	switch v := offer["price"].(type) {
	case float64:
		state.Price = v
	case string:
		if p, ok := parsePrice(v); ok {
			state.Price = p
		}
	}
	if avail, ok := offer["availability"].(string); ok {
		state.InStock = isInStockString(avail)
	}
	return &state
}

func isInStockString(s string) bool {
	lower := strings.ToLower(s)
	// Explicit out-of-stock terms take priority.
	for _, term := range []string{"outofstock", "out_of_stock", "soldout", "sold_out", "discontinued"} {
		if strings.Contains(lower, term) {
			return false
		}
	}
	for _, term := range []string{"instock", "in_stock", "instoreonly", "onlineonly", "limitedavailability", "preorder", "presale", "available"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func parsePrice(s string) (float64, bool) {
	s = strings.TrimSpace(s)

	// If both separators are present, the last one is the decimal separator.
	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")

	switch {
	case lastComma > lastDot:
		// Danish/German style: 1.299,95 — comma is decimal
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	default:
		// English style: 1,299.95 — dot is decimal
		s = strings.ReplaceAll(s, ",", "")
	}

	// Strip any remaining non-numeric characters (currency symbols etc.)
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' {
			return r
		}
		return -1
	}, s)

	p, err := strconv.ParseFloat(s, 64)
	return p, err == nil
}

func processContent(content io.ReadCloser, selector Selector) (string, error) {
	var (
		result string
		err    error
	)
	switch selector.Type {
	case "css":
		result, err = getCSSSelectorContent(content, selector.Paths)
	case "json":
		result, err = getJSONSelectorContent(content, selector.Paths)
	default:
		result, err = getHTMLText(content)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

func filterMatch(filter Filters, content string) bool {
	for _, f := range filter.Contains {
		if strings.Contains(content, f) {
			return true
		}
	}
	for _, f := range filter.NotContains {
		if !strings.Contains(content, f) {
			return true
		}
	}
	return false
}

// GetContent implements MonitorClient for HTTPClient.
func (h *HTTPClient) GetContent(url string, headers http.Header) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http: new request: %w", err)
	}
	req.Header = headers

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("http: unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// GetContent implements MonitorClient for ChromeClient.
func (c *ChromeClient) GetContent(url string, headers http.Header) (io.ReadCloser, error) {
	ctx, cancel := chromedp.NewContext(c.allocCtx)
	defer cancel()

	var actions chromedp.Tasks
	if len(headers) > 0 {
		networkHeaders := make(network.Headers, len(headers))
		for k, vals := range headers {
			networkHeaders[k] = strings.Join(vals, ", ")
		}
		actions = append(actions, network.SetExtraHTTPHeaders(networkHeaders))
	}

	var htmlContent string
	actions = append(actions,
		chromedp.Navigate(url),
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err := chromedp.Run(ctx, actions); err != nil {
		return nil, fmt.Errorf("chromedp: %w", err)
	}
	return io.NopCloser(strings.NewReader(htmlContent)), nil
}

func (c *ChromeClient) close() {
	c.cancelAlloc()
}

func newLocalChromeClient(chromePath string) (*ChromeClient, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	return &ChromeClient{allocCtx: allocCtx, cancelAlloc: cancelAlloc}, nil
}

func newRemoteChromeClient(wsURL string) (*ChromeClient, error) {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	return &ChromeClient{allocCtx: allocCtx, cancelAlloc: cancelAlloc}, nil
}

func getCSSSelectorContent(body io.ReadCloser, selectors []string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", fmt.Errorf("goquery: %w", err)
	}
	results := make([]string, 0, len(selectors))
	for _, sel := range selectors {
		results = append(results, doc.Find(sel).Text())
	}
	return strings.Join(results, "\n"), nil
}

func getHTMLText(body io.ReadCloser) (string, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", fmt.Errorf("goquery: %w", err)
	}
	doc.Find("script").Remove()
	return doc.Find("body").Text(), nil
}

func getJSONSelectorContent(body io.ReadCloser, selectors []string) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("json: read body: %w", err)
	}
	values := gjson.GetManyBytes(data, selectors...)
	results := make([]string, 0, len(values))
	for _, v := range values {
		results = append(results, v.String())
	}
	return strings.Join(results, "\n"), nil
}
