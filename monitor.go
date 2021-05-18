package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
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
	URL         string        `json:"url"`
	HTTPHeaders http.Header   `json:"httpHeaders,omitempty"`
	UseChrome   bool          `json:"useChrome"`
	Selectors   Selectors     `json:"selectors,omitempty"`
	Interval    time.Duration `json:"interval"`
	Notifiers   []string      `json:"notifiers"`
	doneChannel chan bool
	ticker      *time.Ticker
	id          string
	notify      *notify.Notify
	client      MonitorClient
}

type Selectors struct {
	CSS  *string `json:"css,omitempty"`
	JSON *string `json:"json,omitempty"`
}

type Monitors []Monitor

func (m Monitors) StartMonitoring() {
	for idx := range m {
		m[idx].id = generateSHA1String(m[idx].URL)
		m[idx].doneChannel = make(chan bool)
		m[idx].ticker = time.NewTicker(m[idx].Interval * time.Second)

		m[idx].notify = notify.New()

		for _, notifier := range m[idx].Notifiers {
			m[idx].notify.UseServices(notifiers[notifier])
		}

		if m[idx].UseChrome {

		} else {
			m[idx].client = HttpClient{}
		}

		wg.Add(1)
		go func(monitor Monitor) {
			for {
				select {
				case <-monitor.doneChannel:
					wg.Done()
					return
				case <-monitor.ticker.C:
					monitor.check()
				}
			}
		}(m[idx])
	}
}

func generateSHA1String(input string) string {
	h := sha1.New()
	h.Write([]byte(input))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x\n", bs)
}

func (m *Monitor) check() {
	log.Print(m.URL)

	selectorContent := m.client.GetContent(m.URL, m.HTTPHeaders, m.Selectors)

	storage := InitStorage(m)

	if m.compareContent(storage, selectorContent[0]) {
		log.Print("Content has changed!")
	} else {
		_ = m.notify.Send(
			context.Background(),
			"Test Subject",
			"Test Message!",
		)
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

	selectorText := doc.Find(selector).First().Text()

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

func (m *Monitor) compareContent(storage *Storage, content string) bool {
	storageContent := storage.GetContent()
	log.Print("Cache content: " + storageContent)
	log.Print("New content: " + content)

	if storageContent != content {
		storage.WriteContent(content)
		return true
	}

	return false
}
