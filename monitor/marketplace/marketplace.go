package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/PuerkitoBio/goquery"
)

const chromeWaitTimeout = 15 * time.Second

// Listing is a parsed marketplace listing.
type Listing struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Price    string  `json:"price"`
	PriceVal float64 `json:"priceVal,omitempty"`
	URL      string  `json:"url"`
	ImageURL string  `json:"imageUrl,omitempty"`
}

// MarketplaceFeature monitors a marketplace search URL for new listings.
// Selector is a CSS selector that matches individual listing elements on the
// page. The optional *Selector fields let the user pin specific child elements
// for each field; when omitted a simple heuristic is used instead.
type MarketplaceFeature struct {
	// Selector is required: it matches one element per listing on the page.
	Selector string
	// Optional sub-selectors. When empty the extractor falls back to heuristics.
	LinkSelector  string
	TitleSelector string
	PriceSelector string

	Keywords []string
	MaxPrice float64
}

// Check implements monitor.DetectionFeature. It fetches the current listings,
// compares them against the stored set of seen IDs, and sends one notification
// per new listing. On the first run the seen IDs are seeded without any
// notifications.
func (f *MarketplaceFeature) Check(m *monitor.Monitor) {
	listings, err := f.fetchListings(m.Client, m.URL, m.HTTPHeaders)
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
		if err := m.Notifier.Notify(
			context.Background(),
			fmt.Sprintf("ChangeMonitor: %s has a new listing!", m.Name),
			fmt.Sprintf("%s new listing.\n\n---\n(title) %s\n\n(price) %s\n\n(url) %s\n---", m.Name, l.Title, l.Price, l.URL),
		); err != nil {
			log.Printf("marketplace: notify: %v", err)
		}
	}
}

// Preview implements monitor.DetectionFeature. It returns the current listings
// as formatted text without persisting anything.
func (f *MarketplaceFeature) Preview(m monitor.Monitor) (monitor.PreviewResult, error) {
	listings, err := f.fetchListings(m.Client, m.URL, m.HTTPHeaders)
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

// fetchListings retrieves the page HTML (via Chrome or plain HTTP) and extracts
// listings using goquery and the configured CSS selectors.
func (f *MarketplaceFeature) fetchListings(client monitor.MonitorClient, pageURL string, headers http.Header) ([]Listing, error) {
	var r io.ReadCloser

	var err error
	r, err = client.GetContent(pageURL, headers)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}
	defer r.Close()

	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	return f.extractListings(doc, pageURL), nil
}

// extractListings iterates the matched listing elements and converts each one
// into a Listing via the per-field extractors.
func (f *MarketplaceFeature) extractListings(doc *goquery.Document, pageURL string) []Listing {
	seen := make(map[string]bool)
	var listings []Listing

	doc.Find(f.Selector).Each(func(_ int, el *goquery.Selection) {
		listingURL := extractURL(el, f.LinkSelector, pageURL)
		if listingURL == "" || seen[listingURL] {
			return
		}
		seen[listingURL] = true

		title := extractTitle(el, f.TitleSelector)
		priceStr, priceVal := extractPrice(el, f.PriceSelector)
		imageURL, _ := el.Find("img").First().Attr("src")

		listings = append(listings, Listing{
			ID:       listingURL,
			URL:      listingURL,
			Title:    title,
			Price:    priceStr,
			PriceVal: priceVal,
			ImageURL: imageURL,
		})
	})
	return listings
}

// extractURL returns the absolute URL for the listing element.
// Priority: linkSelector → first <a href> in element → href on element itself.
func extractURL(el *goquery.Selection, linkSelector, pageURL string) string {
	var href string
	if linkSelector != "" {
		href, _ = el.Find(linkSelector).First().Attr("href")
	}
	if href == "" {
		if el.Is("a") {
			href, _ = el.Attr("href")
		} else {
			href, _ = el.Find("a").First().Attr("href")
		}
	}
	if href == "" {
		return ""
	}
	return resolveURL(href, pageURL)
}

// extractTitle returns the visible title text for the listing element.
// Priority: titleSelector → first heading/strong → full element text.
func extractTitle(el *goquery.Selection, titleSelector string) string {
	if titleSelector != "" {
		if t := strings.TrimSpace(el.Find(titleSelector).First().Text()); t != "" {
			return t
		}
	}
	if t := strings.TrimSpace(el.Find("h1,h2,h3,h4,h5,strong").First().Text()); t != "" {
		return t
	}
	return strings.TrimSpace(el.Text())
}

// extractPrice returns the price string and its numeric value for the listing.
// Priority: priceSelector → heuristic scan of child text nodes.
func extractPrice(el *goquery.Selection, priceSelector string) (string, float64) {
	if priceSelector != "" {
		if raw := strings.TrimSpace(el.Find(priceSelector).First().Text()); raw != "" {
			v, _ := monitor.ParsePrice(raw)
			return raw, v
		}
	}

	// Walk all text nodes inside the element looking for something price-like.
	var found string
	el.Find("*").Each(func(_ int, child *goquery.Selection) {
		if found != "" {
			return
		}
		// Only consider leaf nodes (no child elements) to avoid duplicate text.
		if child.Children().Length() > 0 {
			return
		}
		text := strings.TrimSpace(child.Text())
		if looksLikePrice(text) {
			found = text
		}
	})

	if found == "" {
		return "", 0
	}
	v, _ := monitor.ParsePrice(found)
	return found, v
}

// resolveURL turns href into an absolute URL using base as the reference.
func resolveURL(href, base string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return baseURL.ResolveReference(ref).String()
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

var currencyCodes = []string{"DKK", "KR"}

func looksLikePrice(s string) bool {
	upper := strings.ToUpper(s)

	hasMarker := strings.ContainsAny(s, "$€£¥")
	if !hasMarker {
		for _, code := range currencyCodes {
			if strings.Contains(upper, code) {
				hasMarker = true
				break
			}
		}
	}

	return hasMarker && strings.ContainsAny(s, "0123456789")
}
