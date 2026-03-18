package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

// rawListing is what the in-page JS extractor produces.
type rawListing struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Text     string `json:"text"`
	ImageURL string `json:"imageUrl"`
}

// Listing is a parsed marketplace listing.
type Listing struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Price    string  `json:"price"`
	PriceVal float64 `json:"priceVal,omitempty"`
	URL      string  `json:"url"`
	ImageURL string  `json:"imageUrl,omitempty"`
}

// Scraper knows how to fetch and parse listings from a specific marketplace site.
type Scraper interface {
	// Fetch retrieves the current listings from the given URL.
	Fetch(client monitor.MonitorClient, url string, headers http.Header) ([]Listing, error)
	// RequiresChrome reports whether this scraper needs a JS-capable browser.
	RequiresChrome() bool
}

// URLRequiresChrome reports whether the marketplace scraper for the given URL
// requires Chrome. Returns false if no scraper matches the URL.
func URLRequiresChrome(u string) bool {
	s, err := scraperForURL(u)
	if err != nil {
		return false
	}
	return s.RequiresChrome()
}

// scraperForURL returns the Scraper appropriate for the given URL.
func scraperForURL(u string) (Scraper, error) {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "facebook.com"):
		return &facebookScraper{}, nil
	case strings.Contains(lower, "dba.dk"):
		return &dbaScraper{}, nil
	default:
		return nil, fmt.Errorf("no marketplace scraper available for URL: %s", u)
	}
}

// MarketplaceFeature monitors a marketplace search URL for new listings.
// The appropriate scraper is chosen automatically based on the URL.
// Keywords filter notifications by title; MaxPrice filters out listings above
// the threshold.
type MarketplaceFeature struct {
	Keywords []string
	MaxPrice float64
}

// Check implements monitor.DetectionFeature. It fetches the current listings,
// compares them against the stored set of seen IDs, and sends one notification
// per new listing. On the first run the seen IDs are seeded without any
// notifications.
func (f *MarketplaceFeature) Check(m *monitor.Monitor) {
	scraper, err := scraperForURL(m.URL)
	if err != nil {
		log.Printf("marketplace: %v", err)
		return
	}

	listings, err := f.fetchListings(m.Client, m.URL, m.HTTPHeaders, scraper)
	if err != nil {
		log.Printf("marketplace: fetch listings: %v", err)
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
		log.Printf("marketplace: initial seed for %q: recorded %d listing(s)", m.Name, len(listings))
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
		log.Printf("marketplace: no new listings for %q, next check in %s", m.Name, m.Interval*time.Minute)
		return
	}

	log.Printf("marketplace: %d new listing(s) for %q", len(newListings), m.Name)
	for _, l := range newListings {
		if f.MaxPrice > 0 && l.PriceVal > f.MaxPrice {
			log.Printf("marketplace: skipping %q (price %.2f > max %.2f)", l.Title, l.PriceVal, f.MaxPrice)
			continue
		}
		if len(f.Keywords) > 0 && !matchesKeywords(l.Title, f.Keywords) {
			log.Printf("marketplace: skipping %q (no keyword match)", l.Title)
			continue
		}
		body := fmt.Sprintf("Price: %s\n%s", l.Price, l.URL)
		if err := m.Notifier.Notify(
			context.Background(),
			fmt.Sprintf("Marketplace: %s - %s", m.Name, l.Title),
			body,
		); err != nil {
			log.Printf("marketplace: notify: %v", err)
		}
	}
}

// Preview implements monitor.DetectionFeature. It returns the current listings
// as formatted text without persisting anything.
func (f *MarketplaceFeature) Preview(m monitor.Monitor) (monitor.PreviewResult, error) {
	scraper, err := scraperForURL(m.URL)
	if err != nil {
		return monitor.PreviewResult{}, err
	}

	listings, err := f.fetchListings(m.Client, m.URL, m.HTTPHeaders, scraper)
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

func (f *MarketplaceFeature) fetchListings(client monitor.MonitorClient, pageURL string, headers http.Header, scraper Scraper) ([]Listing, error) {
	return scraper.Fetch(client, pageURL, headers)
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
