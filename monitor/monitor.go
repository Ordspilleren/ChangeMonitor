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

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// MonitorClient retrieves content from a URL.
type MonitorClient interface {
	GetContent(url string, headers http.Header) (io.ReadCloser, error)
}

// JSEvaluator is an optional extension of MonitorClient that supports
// JavaScript evaluation within a navigated page context.
type JSEvaluator interface {
	EvalOnPage(url string, waitSelector string, timeout time.Duration, jsExpr string) (string, error)
}

// DetectionFeature defines monitor-specific change detection behavior.
// Implementations are responsible for fetching content via m.Client.
type DetectionFeature interface {
	Check(m *Monitor)
	Preview(m Monitor) (PreviewResult, error)
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

// Monitor describes a single URL to be watched for changes.
type Monitor struct {
	Name        string
	URL         string
	HTTPHeaders http.Header
	UseChrome   bool
	Interval    time.Duration
	Feature     DetectionFeature

	Notifier NotifierService
	Storage  Storage
	Client   MonitorClient
	ID       string
	started  bool
	ticker   *time.Ticker
	done     chan struct{}
}

// Monitors is a slice of Monitor values.
type Monitors []Monitor

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
	activeIDs := make([]string, 0, len(ms.monitors))
	for i, m := range ms.monitors {
		id := generateSHA1(m.Name)
		activeIDs = append(activeIDs, id)
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

// PreviewResult holds the outcome of a preview request. Exactly one of Content
// or ProductState will be populated depending on whether product detection is
// enabled.
type PreviewResult struct {
	Content string `json:"content,omitempty"`
}

// Preview fetches and processes content for m without recording anything.
func (ms *MonitorService) Preview(m Monitor) (PreviewResult, error) {
	if m.Feature == nil {
		return PreviewResult{}, fmt.Errorf("preview: no detection feature configured")
	}

	if m.UseChrome {
		if ms.chromeClient == nil {
			return PreviewResult{}, fmt.Errorf("chrome client not initialised")
		}
		m.Client = ms.chromeClient
	} else {
		m.Client = ms.httpClient
	}

	return m.Feature.Preview(m)
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
	m.ID = generateSHA1(m.Name)
	m.done = make(chan struct{}, 1)
	m.ticker = time.NewTicker(m.Interval * time.Minute)
	m.Storage = ms.storage
	m.Notifier = ms.notifier
	if m.UseChrome {
		m.Client = ms.chromeClient
	} else {
		m.Client = ms.httpClient
	}
}

func (m *Monitor) start(wg *sync.WaitGroup) error {
	if m.started {
		return errors.New("monitor is already started")
	}
	if m.Feature == nil {
		return errors.New("no detection feature configured")
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
	log.Printf("monitor: checking %s", m.Name)
	if m.Feature == nil {
		log.Printf("monitor: no detection feature configured for %q", m.Name)
		return
	}
	m.Feature.Check(m)
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

// EvalOnPage implements JSEvaluator for ChromeClient. It navigates to url,
// polls for waitSelector to appear (up to timeout), then evaluates jsExpr and
// returns the result string. A timeout waiting for the selector is treated as
// non-fatal so that pages with no matching elements still evaluate normally.
func (c *ChromeClient) EvalOnPage(url, waitSelector string, timeout time.Duration, jsExpr string) (string, error) {
	ctx, cancel := chromedp.NewContext(c.allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancelTimeout()

	tasks := chromedp.Tasks{chromedp.Navigate(url)}
	if waitSelector != "" && timeout > 0 {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				var count int
				if err := chromedp.Evaluate(
					fmt.Sprintf(`document.querySelectorAll(%q).length`, waitSelector),
					&count,
				).Do(ctx); err == nil && count > 0 {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
				}
			}
			return nil // selector timeout is non-fatal; proceed with evaluation
		}))
	}

	var result string
	tasks = append(tasks, chromedp.Evaluate(jsExpr, &result))

	if err := chromedp.Run(ctx, tasks...); err != nil {
		return "", fmt.Errorf("chromedp eval: %w", err)
	}
	return result, nil
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
