package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

const (
	waitSelector = `a[href*="/marketplace/item/"]`
	waitTimeout  = 15 * time.Second
)

// rawListing is what the in-page JS extractor produces.
type rawListing struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Text     string `json:"text"`
	ImageURL string `json:"imageUrl"`
}

// Listing is a parsed Facebook Marketplace listing.
type Listing struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Price    string  `json:"price"`
	PriceVal float64 `json:"priceVal,omitempty"`
	URL      string  `json:"url"`
	ImageURL string  `json:"imageUrl,omitempty"`
}

// FacebookFeature monitors a Facebook Marketplace search URL for new listings.
// The user provides the full URL. Keywords are used to filter notifications by title;
// MaxPrice filters out listings above the threshold.
type FacebookFeature struct {
	Keywords []string
	MaxPrice float64
}

// Check implements monitor.DetectionFeature. It fetches the current listings
// via JS evaluation, compares them against the stored set of seen IDs, and
// sends one notification per new listing. On the first run the seen IDs are
// seeded without any notifications.
func (f *FacebookFeature) Check(m *monitor.Monitor) {
	listings, err := f.fetchListings(m.Client, m.URL)
	if err != nil {
		log.Printf("facebook: fetch listings: %v", err)
		return
	}

	storedJSON := m.Storage.GetContent(m.ID)
	if storedJSON == "" {
		// First run: record current listings without notifying.
		ids := make([]string, 0, len(listings))
		for _, l := range listings {
			ids = append(ids, l.ID)
		}
		seedJSON, _ := json.Marshal(ids)
		m.Storage.WriteContent(m.ID, string(seedJSON))
		log.Printf("facebook: initial seed for %q: recorded %d listing(s)", m.Name, len(listings))
		return
	}

	seen := make(map[string]struct{})
	var storedIDs []string
	if err := json.Unmarshal([]byte(storedJSON), &storedIDs); err == nil {
		for _, id := range storedIDs {
			seen[id] = struct{}{}
		}
	}

	var newListings []Listing
	for _, l := range listings {
		if _, known := seen[l.ID]; !known {
			newListings = append(newListings, l)
			seen[l.ID] = struct{}{}
		}
	}

	updatedIDs := make([]string, 0, len(seen))
	for id := range seen {
		updatedIDs = append(updatedIDs, id)
	}
	updatedJSON, _ := json.Marshal(updatedIDs)
	m.Storage.WriteContent(m.ID, string(updatedJSON))

	if len(newListings) == 0 {
		log.Printf("facebook: no new listings for %q, next check in %s", m.Name, m.Interval*time.Minute)
		return
	}

	log.Printf("facebook: %d new listing(s) for %q", len(newListings), m.Name)
	for _, l := range newListings {
		if f.MaxPrice > 0 && l.PriceVal > f.MaxPrice {
			log.Printf("facebook: skipping %q (price %.2f > max %.2f)", l.Title, l.PriceVal, f.MaxPrice)
			continue
		}
		if len(f.Keywords) > 0 && !matchesKeywords(l.Title, f.Keywords) {
			log.Printf("facebook: skipping %q (no keyword match)", l.Title)
			continue
		}
		body := fmt.Sprintf("Price: %s\n%s", l.Price, l.URL)
		if err := m.Notifier.Notify(
			context.Background(),
			fmt.Sprintf("Facebook Marketplace: %s - %s", m.Name, l.Title),
			body,
		); err != nil {
			log.Printf("facebook: notify: %v", err)
		}
	}
}

// Preview implements monitor.DetectionFeature. It returns the current
// listings as formatted text without persisting anything.
func (f *FacebookFeature) Preview(m monitor.Monitor) (monitor.PreviewResult, error) {
	listings, err := f.fetchListings(m.Client, m.URL)
	if err != nil {
		return monitor.PreviewResult{}, err
	}
	if len(listings) == 0 {
		return monitor.PreviewResult{Content: "No listings found."}, nil
	}
	var sb strings.Builder
	for i, l := range listings {
		fmt.Fprintf(&sb, "%d. %s - %s\n   %s\n", i+1, l.Title, l.Price, l.URL)
	}
	return monitor.PreviewResult{Content: strings.TrimSpace(sb.String())}, nil
}

func (f *FacebookFeature) fetchListings(client monitor.MonitorClient, pageURL string) ([]Listing, error) {
	jsClient, ok := client.(monitor.JSEvaluator)
	if !ok {
		return nil, fmt.Errorf("client does not support JavaScript evaluation; set useChrome: true")
	}

	result, err := jsClient.EvalOnPage(pageURL, waitSelector, waitTimeout, extractJS)
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
		title, priceStr, priceVal := parseText(r.Text)
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

// extractJS is evaluated inside the browser to harvest listing data from the DOM.
const extractJS = `
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

var (
	pricePattern = regexp.MustCompile(`^\$[\d,]+(?:\.\d{2})?`)
	digitsOnly   = regexp.MustCompile(`[\d,.]+`)
)

// parseText extracts title, price string, and numeric price from the raw
// innerText of a listing card. Facebook listing cards look roughly like:
//
//	$250\nMacBook Pro 2019\nSeattle, WA · Listed a day ago
func parseText(raw string) (title, priceStr string, priceVal float64) {
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
		numStr := strings.ReplaceAll(digitsOnly.FindString(priceStr), ",", "")
		priceVal, _ = strconv.ParseFloat(numStr, 64)
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

// matchesKeywords reports whether title contains at least one keyword (case-insensitive).
func matchesKeywords(title string, keywords []string) bool {
	lower := strings.ToLower(title)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
