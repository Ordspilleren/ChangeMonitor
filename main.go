package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"sync"
	"time"
)

var wg = &sync.WaitGroup{}

type Config struct {
	Monitors []Monitor `json:"monitors"`
}

type Monitor struct {
	URL         string        `json:"url"`
	CSSSelector string        `json:"cssSelector"`
	Interval    time.Duration `json:"interval"`
	doneChannel chan bool
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
	for _, monitor := range monitors {
		monitor.doneChannel = make(chan bool)
		monitorTicker := time.NewTicker(monitor.Interval * time.Second)
		wg.Add(1)
		go func(monitor Monitor) {
			for {
				select {
				case <-monitor.doneChannel:
					wg.Done()
					return
				case <-monitorTicker.C:
					err = monitor.check()
					if err != nil {
						monitor.doneChannel <- true
					}
				}
			}
		}(monitor)
	}
	return
}

func (m *Monitor) check() (err error) {
	log.Print(m.URL)
	return
}
