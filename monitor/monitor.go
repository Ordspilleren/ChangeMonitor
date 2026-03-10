package monitor

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

// Monitor describes a single URL to be watched for changes.
type Monitor struct {
	Name        string        `json:"name"`
	URL         string        `json:"url"`
	HTTPHeaders http.Header   `json:"httpHeaders,omitempty"`
	UseChrome   bool          `json:"useChrome"`
	Interval    time.Duration `json:"interval"`
	Selector    Selector      `json:"selector,omitempty"`
	Filters     *Filters      `json:"filters,omitempty"`
	IgnoreEmpty bool          `json:"ignoreEmpty,omitempty"`

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

// Start initializes every monitor and begins polling. It returns immediately;
// monitors run in background goroutines.
func (ms *MonitorService) Start() {
	for i := range ms.monitors {
		ms.monitors[i].init(ms)
		if err := ms.monitors[i].start(&ms.wg); err != nil {
			log.Printf("monitor: failed to start %q: %v", ms.monitors[i].Name, err)
		}
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
	m.id = generateSHA1(m.URL)
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
		fmt.Sprintf("<b><u>%s has changed!</u></b>", m.Name),
		fmt.Sprintf("<b>New content:</b>\n%.200s\n\n<b>Old content:</b>\n%.200s\n\n<b>URL:</b> %s", processed, stored, m.URL),
	); err != nil {
		log.Printf("monitor: notify: %v", err)
	}
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
