package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/nikoksr/notify"
	"github.com/tidwall/gjson"
)

type MonitorClient interface {
	GetContent(url string, httpHeaders http.Header, selectors Selectors) []string
}

type ChromeClient struct {
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
	Notifiers   []string      `json:"notifiers"`
	doneChannel chan bool
	ticker      *time.Ticker
	id          string
	notifier    *notify.Notify
	client      MonitorClient
	storage     *Storage
}

type Selectors struct {
	CSS  *string `json:"css,omitempty"`
	JSON *string `json:"json,omitempty"`
}

type Monitors []Monitor

func (m *Monitor) Init(notifierMap NotifierMap) {
	m.id = generateSHA1String(m.URL)
	m.doneChannel = make(chan bool)
	m.ticker = time.NewTicker(m.Interval * time.Minute)
	m.storage = InitStorage(m.id)

	m.notifier = notify.New()

	for _, notifier := range m.Notifiers {
		m.notifier.UseServices(notifierMap[notifier])
	}

	if m.UseChrome {

	} else {
		m.client = HttpClient{}
	}
}

func (m *Monitor) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func(monitor *Monitor) {
		monitor.check()
		for {
			select {
			case <-monitor.doneChannel:
				wg.Done()
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

func (m Monitors) StartMonitoring(wg *sync.WaitGroup, notifierMap NotifierMap) {
	for idx := range m {
		m[idx].Init(notifierMap)
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

	selectorContent := m.client.GetContent(m.URL, m.HTTPHeaders, m.Selectors)
	storageContent := m.storage.GetContent()

	if m.compareContent(storageContent, selectorContent[0]) {
		m.storage.WriteContent(selectorContent[0])
		log.Print("Content has changed!")
		_ = m.notifier.Send(
			context.Background(),
			fmt.Sprintf("%s has changed!", m.Name),
			fmt.Sprintf("New content: %s\nOld content: %s\nURL: %s", selectorContent[0], storageContent, m.URL),
		)
	} else {
		log.Printf("Nothing has changed, waiting %s.", m.Interval*time.Minute)
	}
}

func (h HttpClient) GetContent(url string, httpHeaders http.Header, selectors Selectors) []string {
	responseBody := getHTTPBody(url, httpHeaders)

	var selectorContent string

	if selectors.CSS != nil {
		selectorContent = getCSSSelectorContent(responseBody, *selectors.CSS)
	} else if selectors.JSON != nil {
		selectorContent = getJSONSelectorContent(responseBody, *selectors.JSON)
	} else {
		bodyBytes, err := ioutil.ReadAll(responseBody)
		if err != nil {
			log.Fatal(err)
		}
		selectorContent = string(bodyBytes)
	}
	return []string{selectorContent}
}

func getHTTPBody(url string, headers http.Header) io.ReadCloser {
	httpClient := http.Client{}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}

	request.Header = headers

	response, err := httpClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}

	return response.Body
}

func getCSSSelectorContent(body io.ReadCloser, selector string) string {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		log.Fatal(err)
	}

	selectorText := doc.Find(selector).Text()

	return selectorText
}

func getJSONSelectorContent(body io.ReadCloser, selector string) string {
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		log.Fatal(err)
	}

	jsonValue := gjson.GetBytes(bodyBytes, selector)
	return jsonValue.String()
}

func (m *Monitor) compareContent(storage string, selector string) bool {
	log.Print("Cache content: " + storage)
	log.Print("New content: " + selector)

	if storage != selector {
		return true
	}

	return false
}
