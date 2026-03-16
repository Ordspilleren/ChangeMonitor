package monitor

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

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

func (m *Monitor) checkGeneric(content io.ReadCloser, selector Selector) {
	processed, err := processContent(content, selector)
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
