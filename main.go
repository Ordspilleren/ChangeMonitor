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
)

var wg = &sync.WaitGroup{}

type Config struct {
	Monitors []Monitor `json:"monitors"`
}

type Monitor struct {
	URL         string        `json:"url"`
	UseChrome   bool          `json:"useChrome"`
	CSSSelector string        `json:"cssSelector"`
	Interval    time.Duration `json:"interval"`
	doneChannel chan bool
	ticker      *time.Ticker
	id          string
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

	if m.UseChrome {
		log.Print("Using Chrome backend")
	} else {
		var headers http.Header
		responseBody := getHTTPBody(m.URL, headers)
		selectorContent := getCSSSelectorContent(responseBody, m.CSSSelector)
		compareContent(m.id, selectorContent)
	}

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
	//defer response.Body.Close()

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

func compareContent(monitorId string, content string) {
	dataDir := "data"
	filePath := filepath.Join(dataDir, monitorId)

	os.Mkdir(dataDir, os.ModePerm)

	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err := ioutil.WriteFile(filePath, []byte(content), 0644)
			if err != nil {
				log.Panic(err)
			}
			log.Print("File did not exist, creating and doing nothing...")
			return
		}
	}

	if string(fileData) == content {
		log.Print("No changes.")
		return
	} else {
		log.Print("Content has changed!")
		err := ioutil.WriteFile(filePath, []byte(content), 0644)
		if err != nil {
			log.Panic(err)
		}
	}
}
