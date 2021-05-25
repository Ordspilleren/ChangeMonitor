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

	"github.com/Ordspilleren/ChangeMonitor/notify"
	"github.com/Ordspilleren/ChangeMonitor/storage"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/tidwall/gjson"
)

type MonitorClient interface {
	GetContent(url string, httpHeaders http.Header, selectors Selectors) (string, error)
}

type ChromeClient struct {
	Path string
}

type HttpClient struct {
}

type Monitor struct {
	Name        string        `json:"name"`
	URL         string        `json:"url"`
	HTTPHeaders http.Header   `json:"httpHeaders,omitempty"`
	UseChrome   bool          `json:"useChrome"`
	Selectors   Selectors     `json:"selectors,omitempty"`
	Interval    time.Duration `json:"interval"`
	Notifiers   []string      `json:"notifiers,omitempty"`
	started     bool
	doneChannel chan bool
	ticker      *time.Ticker
	id          string
	notifiers   notify.NotifierList
	client      MonitorClient
	storage     *storage.Storage
}

type Selectors struct {
	CSS  *[]string `json:"css,omitempty"`
	JSON *[]string `json:"json,omitempty"`
}

type Monitors []Monitor

func NewMonitor(name string, url string, interval int64, notifiers []string) *Monitor {
	monitor := &Monitor{
		Name:      name,
		URL:       url,
		UseChrome: false,
		Interval:  time.Duration(interval),
		Notifiers: notifiers,
	}

	return monitor
}

func (m *Monitor) AddCSSSelectors(selectors ...string) {
	m.Selectors.CSS = &selectors
}

func (m *Monitor) AddJSONSelectors(selectors ...string) {
	m.Selectors.JSON = &selectors
}

func (m *Monitor) Init(notifierMap notify.NotifierMap, storageDirectory string, chromePath string) {
	m.id = generateSHA1String(m.URL)
	m.doneChannel = make(chan bool)
	m.ticker = time.NewTicker(m.Interval * time.Minute)
	m.storage = storage.InitStorage(m.id, storageDirectory)

	for _, notifier := range m.Notifiers {
		m.notifiers = append(m.notifiers, notifierMap[notifier])
	}

	if m.UseChrome {
		m.client = ChromeClient{Path: chromePath}
	} else {
		m.client = HttpClient{}
	}
}

func (m *Monitor) Start(wg *sync.WaitGroup) {
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
}

func (m *Monitor) Stop() {
	m.doneChannel <- true
}

func (m *Monitor) IsRunning() bool {
	return m.started
}

func (m Monitors) StartMonitoring(wg *sync.WaitGroup, notifierMap notify.NotifierMap, storageDirectory string, chromePath string) {
	for idx := range m {
		m[idx].Init(notifierMap, storageDirectory, chromePath)
		m[idx].Start(wg)
	}
}

func generateSHA1String(input string) string {
	h := sha1.New()
	h.Write([]byte(input))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)
}

func (m *Monitor) check() {
	log.Print(m.URL)

	selectorContent, err := m.client.GetContent(m.URL, m.HTTPHeaders, m.Selectors)
	if err != nil {
		log.Print(err)
		return
	}
	storageContent := m.storage.GetContent()

	if m.compareContent(storageContent, selectorContent) {
		m.storage.WriteContent(selectorContent)
		log.Print("Content has changed!")
		_ = m.notifiers.Send(
			context.Background(),
			fmt.Sprintf("<b><u>%s has changed!</u></b>", m.Name),
			fmt.Sprintf("<b>New content:</b>\n%.200s\n\n<b>Old content:</b>\n%.200s\n\n<b>URL:</b> %s", selectorContent, storageContent, m.URL),
		)
	} else {
		log.Printf("Nothing has changed, waiting %s.", m.Interval*time.Minute)
	}
}

func (h HttpClient) GetContent(url string, httpHeaders http.Header, selectors Selectors) (string, error) {
	responseBody, err := getHTTPBody(url, httpHeaders)
	if err != nil {
		return "", err
	}
	defer responseBody.Close()

	var selectorContent string

	if selectors.CSS != nil {
		selectorContent, err = getCSSSelectorContent(responseBody, *selectors.CSS)
		if err != nil {
			return "", err
		}
	} else if selectors.JSON != nil {
		selectorContent, err = getJSONSelectorContent(responseBody, *selectors.JSON)
		if err != nil {
			return "", err
		}
	} else {
		selectorContent, err = getHTMLText(responseBody)
		if err != nil {
			return "", err
		}
	}

	return selectorContent, nil
}

func getHTTPBody(url string, headers http.Header) (io.ReadCloser, error) {
	httpClient := http.Client{}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %v", err)
	}

	request.Header = headers

	response, err := httpClient.Do(request)
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

func (m *Monitor) compareContent(storage string, selector string) bool {
	log.Printf("Cache content: %s", storage)
	log.Printf("New content: %s", selector)

	return storage != selector
}

func (h ChromeClient) GetContent(url string, httpHeaders http.Header, selectors Selectors) (string, error) {
	launcher := launcher.New().Bin(h.Path)
	u, err := launcher.Launch()
	if err != nil {
		return "", fmt.Errorf("failed to launch browser: %v", err)
	}
	defer launcher.Cleanup()

	browser := rod.New().ControlURL(u)
	err = browser.Connect()
	if err != nil {
		return "", fmt.Errorf("failed to connect to browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return "", fmt.Errorf("unable to connect to url: %v", err)
	}
	wait := page.MustWaitRequestIdle()
	wait()
	pageContent, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("unable to fetch html content: %v", err)
	}

	reader := strings.NewReader(pageContent)
	readCloser := io.NopCloser(reader)
	defer readCloser.Close()

	var selectorContent string

	if selectors.CSS != nil {
		selectorContent, err = getCSSSelectorContent(readCloser, *selectors.CSS)
		if err != nil {
			return "", err
		}
	} else if selectors.JSON != nil {
		selectorContent, err = getJSONSelectorContent(readCloser, *selectors.JSON)
		if err != nil {
			return "", err
		}
	} else {
		selectorContent, err = getHTMLText(readCloser)
		if err != nil {
			return "", err
		}
	}

	return selectorContent, nil
}
