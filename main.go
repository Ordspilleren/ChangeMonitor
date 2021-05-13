package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

var wg = &sync.WaitGroup{}

type Config struct {
	Monitors []Monitor `json:"monitors"`
}

type Monitor struct {
	URL          string        `json:"url"`
	HTTPHeaders  http.Header   `json:"httpHeaders,omitempty"`
	UseChrome    bool          `json:"useChrome"`
	CSSSelector  *string       `json:"cssSelector,omitempty"`
	JSONSelector *string       `json:"jsonSelector,omitempty"`
	Interval     time.Duration `json:"interval"`
	doneChannel  chan bool
	ticker       *time.Ticker
	id           string
}

var config Config

func main() {
	b, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Print(err)
		return
	}
	err = json.Unmarshal(b, &config)
	if err != nil {
		log.Print(err)
		return
	}

	err = StartMonitoring(config.Monitors)
	if err != nil {
		log.Print(err)
		return
	}

	wg.Wait()
}

func StartMonitoring(monitors []Monitor) (err error) {
	for idx := range monitors {
		monitors[idx].id = generateSHA1String(monitors[idx].URL)
		monitors[idx].doneChannel = make(chan bool)
		monitors[idx].ticker = time.NewTicker(monitors[idx].Interval * time.Second)
		wg.Add(1)
		go func(monitor Monitor) {
			for {
				select {
				case <-monitor.doneChannel:
					wg.Done()
					return
				case <-monitor.ticker.C:
					err = monitor.check()
					if err != nil {
						monitor.doneChannel <- true
					}
				}
			}
		}(monitors[idx])
	}
	return
}

func generateSHA1String(input string) string {
	h := sha1.New()
	h.Write([]byte(input))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x\n", bs)
}

func (m *Monitor) check() (err error) {
	log.Print(m.URL)

	responseBody := getHTTPBody(m.URL, m.HTTPHeaders)

	var selectorContent string

	if m.CSSSelector != nil {
		selectorContent = getCSSSelectorContent(responseBody, *m.CSSSelector)
	} else if m.JSONSelector != nil {
		selectorContent = getJSONSelectorContent(responseBody, *m.JSONSelector)
	} else {
		bodyBytes, err := ioutil.ReadAll(responseBody)
		if err != nil {
			log.Fatal(err)
		}
		selectorContent = string(bodyBytes)
	}

	compareContent(m.id, selectorContent)

	return
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

func compareContent(monitorId string, content string) {
	cacheContent := getCacheContent(monitorId)

	log.Print("Cache content: " + cacheContent)
	log.Print("New content: " + content)

	if cacheContent == content {
		log.Print("No changes.")
		return
	} else {
		log.Print("Content has changed!")
		writeCacheContent(monitorId, content)
	}
}

func getCacheContent(monitorId string) string {
	dataDir := "data"
	filePath := filepath.Join(dataDir, monitorId)

	os.Mkdir(dataDir, os.ModePerm)

	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Print("File did not exist, returning empty string.")
			return ""
		}
	}

	return string(fileData)
}

func writeCacheContent(monitorId string, content string) {
	dataDir := "data"
	filePath := filepath.Join(dataDir, monitorId)

	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}
