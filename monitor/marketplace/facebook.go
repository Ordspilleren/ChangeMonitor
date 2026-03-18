package marketplace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

const (
	facebookWaitSelector = `a[href*="/marketplace/item/"]`
	facebookWaitTimeout  = 15 * time.Second
)

type facebookScraper struct{}

func (s *facebookScraper) RequiresChrome() bool { return true }

func (s *facebookScraper) Fetch(client monitor.MonitorClient, url string, headers http.Header) ([]Listing, error) {
	jsClient, ok := client.(monitor.JSEvaluator)
	if !ok {
		return nil, fmt.Errorf("client does not support JavaScript evaluation; set useChrome: true")
	}
	result, err := jsClient.EvalOnPage(url, facebookWaitSelector, facebookWaitTimeout, facebookExtractJS)
	if err != nil {
		return nil, fmt.Errorf("eval on page: %w", err)
	}
	if result == "" {
		return nil, nil
	}
	var raw []rawListing
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}
	listings := make([]Listing, 0, len(raw))
	for _, r := range raw {
		title, priceStr, priceVal := parseFacebookText(r.Text)
		if title == "" {
			continue
		}
		listings = append(listings, Listing{
			ID:       r.ID,
			Title:    title,
			Price:    priceStr,
			PriceVal: priceVal,
			URL:      r.URL,
			ImageURL: r.ImageURL,
		})
	}
	return listings, nil
}

// facebookExtractJS is evaluated inside the browser to harvest listing data from the DOM.
const facebookExtractJS = `
(() => {
const results = [];
const seen = new Set();
const links = document.querySelectorAll('a[href*="/marketplace/item/"]');
for (const el of links) {
  try {
    const href = el.href;
    const m = href.match(/\/marketplace\/item\/(\d+)/);
    if (!m) continue;
    const id = m[1];
    if (seen.has(id)) continue;
    seen.add(id);
    const img = el.querySelector('img');
    results.push({ id, url: href, text: el.innerText, imageUrl: img ? img.src : '' });
  } catch(_) {}
}
return JSON.stringify(results);
})()`

var pricePattern = regexp.MustCompile(`^\$[\d,]+(?:\.\d{2})?`)

// parseFacebookText extracts title, price string, and numeric price from the
// raw innerText of a Facebook Marketplace listing card. Cards look roughly like:
//
//	$250\nMacBook Pro 2019\nSeattle, WA · Listed a day ago
func parseFacebookText(raw string) (title, priceStr string, priceVal float64) {
	lines := splitLines(raw)

	priceIdx := -1
	for i, line := range lines {
		if pricePattern.MatchString(line) || strings.EqualFold(strings.TrimSpace(line), "free") {
			priceStr = strings.TrimSpace(line)
			priceIdx = i
			break
		}
	}

	if priceStr != "" && !strings.EqualFold(priceStr, "free") {
		priceVal, _ = monitor.ParsePrice(priceStr)
	}

	for i, line := range lines {
		if i == priceIdx {
			continue
		}
		// Skip location/timestamp lines (contain middle-dot or end with "ago").
		if strings.Contains(line, "\u00b7") || strings.HasSuffix(line, "ago") {
			continue
		}
		if line != "" {
			title = line
			break
		}
	}
	return
}

func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}
