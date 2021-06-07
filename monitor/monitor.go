package monitor

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/tidwall/gjson"
)

type MonitorClient interface {
	GetContent(url string, httpHeaders http.Header) (io.ReadCloser, error)
}

type Storage interface {
	GetContent(id string) string
	WriteContent(id string, content string)
}

type NotifierService interface {
	Send(ctx context.Context, subject, message string) error
}

type MonitorService struct {
	WaitGroup       *sync.WaitGroup
	Monitors        Monitors
	HttpClient      *HttpClient
	ChromeClient    *ChromeClient
	Storage         *Storage
	NotifierService *NotifierService
}

type ChromeClient struct {
	Launcher *launcher.Launcher
	Browser  *rod.Browser
}

type HttpClient struct {
	Client http.Client
}

type Monitor struct {
	Name            string        `json:"name"`
	URL             string        `json:"url"`
	HTTPHeaders     http.Header   `json:"httpHeaders,omitempty"`
	UseChrome       bool          `json:"useChrome"`
	Interval        time.Duration `json:"interval"`
	Selectors       Selectors     `json:"selectors,omitempty"`
	Filters         Filters       `json:"filters,omitempty"`
	notifierService NotifierService
	started         bool
	doneChannel     chan bool
	ticker          *time.Ticker
	id              string
	client          MonitorClient
	storage         Storage
}

type Monitors []Monitor

type Selectors struct {
	CSS  *[]string `json:"css,omitempty"`
	JSON *[]string `json:"json,omitempty"`
}

type Filters []string

func NewMonitorService(wg *sync.WaitGroup, monitors Monitors, storage Storage, notifierService NotifierService) *MonitorService {
	monitorService := MonitorService{
		WaitGroup:       wg,
		Monitors:        monitors,
		Storage:         &storage,
		NotifierService: &notifierService,
	}

	return &monitorService
}

func (ms *MonitorService) NewMonitorClients(chromePath string, externalClient bool) error {
	var usingChrome bool

	for _, monitor := range ms.Monitors {
		if monitor.UseChrome {
			usingChrome = true
			break
		}
	}

	if usingChrome {
		chromeClient, err := newChromeClient(chromePath, externalClient)
		if err != nil {
			return fmt.Errorf("failed to create new chrome client: %v", err)
		}
		ms.ChromeClient = chromeClient
	} else {
		ms.ChromeClient = &ChromeClient{}
	}

	ms.HttpClient = &HttpClient{Client: http.Client{}}

	return nil
}

func (ms *MonitorService) InitMonitors() error {
	for idx := range ms.Monitors {
		ms.Monitors[idx].Init(*ms.NotifierService, *ms.Storage, *ms.HttpClient, *ms.ChromeClient)
	}

	return nil
}

func (ms *MonitorService) StartMonitoring() error {
	for idx := range ms.Monitors {
		ms.Monitors[idx].Start(ms.WaitGroup)
	}

	return nil
}

func (ms *MonitorService) AddMonitors(monitors ...Monitor) error {
	ms.Monitors = append(ms.Monitors, monitors...)

	return nil
}

func NewMonitor(name string, url string, interval int64) *Monitor {
	monitor := &Monitor{
		Name:      name,
		URL:       url,
		UseChrome: false,
		Interval:  time.Duration(interval),
	}

	return monitor
}

func (m *Monitor) AddCSSSelectors(selectors ...string) {
	m.Selectors.CSS = &selectors
}

func (m *Monitor) AddJSONSelectors(selectors ...string) {
	m.Selectors.JSON = &selectors
}

func (m *Monitor) Init(notifierService NotifierService, storage Storage, httpClient HttpClient, chromeClient ChromeClient) {
	m.id = generateSHA1String(m.URL)
	m.doneChannel = make(chan bool)
	m.ticker = time.NewTicker(m.Interval * time.Minute)
	m.storage = storage
	m.notifierService = notifierService

	if m.UseChrome {
		m.client = chromeClient
	} else {
		m.client = httpClient
	}
}

func (m *Monitor) Start(wg *sync.WaitGroup) error {
	if m.started {
		return errors.New("monitor is already started")
	}

	wg.Add(1)
	m.started = true
	go func(monitor *Monitor) {
		monitor.check()
		for {
			select {
			case <-monitor.doneChannel:
				wg.Done()
				m.started = false
				return
			case <-monitor.ticker.C:
				monitor.check()
			}
		}
	}(m)

	return nil
}

func (m *Monitor) Stop() {
	m.started = false
	m.doneChannel <- true
}

func (m *Monitor) IsRunning() bool {
	return m.started
}

func generateSHA1String(input string) string {
	h := sha1.New()
	h.Write([]byte(input))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)
}

func (m *Monitor) check() {
	log.Print(m.URL)

	content, err := m.client.GetContent(m.URL, m.HTTPHeaders)
	if err != nil {
		log.Print(err)
		return
	}
	defer content.Close()

	processedContent, err := processContent(content, m.Selectors, m.Filters)
	if err != nil {
		log.Print(err)
		return
	}

	storageContent := m.storage.GetContent(m.id)

	if compareContent(storageContent, processedContent) {
		m.storage.WriteContent(m.id, processedContent)
		log.Print("Content has changed!")
		_ = m.notifierService.Send(
			context.Background(),
			fmt.Sprintf("<b><u>%s has changed!</u></b>", m.Name),
			fmt.Sprintf("<b>New content:</b>\n%.200s\n\n<b>Old content:</b>\n%.200s\n\n<b>URL:</b> %s", processedContent, storageContent, m.URL),
		)
	} else {
		log.Printf("Nothing has changed, waiting %s.", m.Interval*time.Minute)
	}
}

func processContent(content io.ReadCloser, selectors Selectors, filters Filters) (string, error) {
	var selectorContent string
	var err error

	if selectors.CSS != nil {
		selectorContent, err = getCSSSelectorContent(content, *selectors.CSS)
		if err != nil {
			return "", err
		}
	} else if selectors.JSON != nil {
		selectorContent, err = getJSONSelectorContent(content, *selectors.JSON)
		if err != nil {
			return "", err
		}
	} else {
		selectorContent, err = getHTMLText(content)
		if err != nil {
			return "", err
		}
	}

	if len(filters) > 0 {
		selectorContent = processFilters(filters, selectorContent)
	}

	return selectorContent, nil
}

func processFilters(filters Filters, content string) string {
	var matchedFilters []string

	for _, filter := range filters {
		if strings.Contains(content, filter) {
			matchedFilters = append(matchedFilters, filter)
		}
	}

	if len(matchedFilters) > 0 {
		return strings.Join(matchedFilters, " | ")
	} else {
		return ""
	}
}

func (h HttpClient) GetContent(url string, httpHeaders http.Header) (io.ReadCloser, error) {
	responseBody, err := h.getHTTPBody(url, httpHeaders)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

func (h HttpClient) getHTTPBody(url string, headers http.Header) (io.ReadCloser, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %v", err)
	}

	request.Header = headers

	response, err := h.Client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to complete http request: %v", err)
	}

	if response.StatusCode != 200 {
		return nil, errors.New("http response is not 200 ok")
	}

	return response.Body, nil
}

func getCSSSelectorContent(body io.ReadCloser, selectors []string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", fmt.Errorf("failed to create goquery document: %v", err)
	}

	var results []string
	for _, selector := range selectors {
		results = append(results, doc.Find(selector).Text())
	}

	return strings.Join(results, "\n"), nil
}

func getHTMLText(body io.ReadCloser) (string, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", fmt.Errorf("failed to create goquery document: %v", err)
	}

	doc.Find("script").Remove()

	return doc.Find("body").Text(), nil
}

func getJSONSelectorContent(body io.ReadCloser, selectors []string) (string, error) {
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("failed to read body during getting json selector content: %v", err)
	}

	var results []string
	result := gjson.GetManyBytes(bodyBytes, selectors...)

	for _, value := range result {
		results = append(results, value.String())
	}

	return strings.Join(results, "\n"), nil
}

func compareContent(storage string, selector string) bool {
	log.Printf("Cache content: %s", storage)
	log.Printf("New content: %s", selector)

	return storage != selector
}

func (h ChromeClient) GetContent(url string, httpHeaders http.Header) (io.ReadCloser, error) {
	responseBody, err := h.getHTTPBody(url, httpHeaders)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

func (h ChromeClient) getHTTPBody(url string, headers http.Header) (io.ReadCloser, error) {
	page, err := h.Browser.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("unable to connect to url: %v", err)
	}
	defer page.Close()

	wait := page.MustWaitRequestIdle()
	wait()
	pageContent, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("unable to fetch html content: %v", err)
	}

	reader := strings.NewReader(pageContent)
	readCloser := io.NopCloser(reader)

	return readCloser, nil
}

func newChromeClient(chromePath string, externalClient bool) (*ChromeClient, error) {
	chromeClient := &ChromeClient{}
	var url string
	var err error

	if externalClient {
		url, err = launcher.ResolveURL(chromePath)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to browser: %v", err)
		}
	} else {
		chromeClient.Launcher = launcher.New().Bin(chromePath)
		url, err = chromeClient.Launcher.Launch()
		if err != nil {
			return nil, fmt.Errorf("failed to launch browser: %v", err)
		}
	}

	chromeClient.Browser = rod.New().ControlURL(url)
	err = chromeClient.Browser.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %v", err)
	}

	return chromeClient, nil
}
