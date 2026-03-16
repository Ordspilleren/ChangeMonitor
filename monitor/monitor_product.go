package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ProductDetection configures automatic stock and price change detection for
// single-product pages. When enabled, the normal content-hash check is
// replaced by a structured product-state comparison.
type ProductDetection struct {
	TrackStock bool     `json:"trackStock,omitempty"`
	TrackPrice bool     `json:"trackPrice,omitempty"`
	MinPrice   *float64 `json:"minPrice,omitempty"`
	MaxPrice   *float64 `json:"maxPrice,omitempty"`
}

// ProductState holds the last-observed price and availability for a product monitor.
type ProductState struct {
	InStock bool    `json:"inStock"`
	Price   float64 `json:"price"`
}

func (m *Monitor) checkProduct(content io.ReadCloser) {
	body, err := io.ReadAll(content)
	if err != nil {
		log.Printf("monitor: product detection: read body: %v", err)
		return
	}

	current, err := extractProductData(body)
	if err != nil {
		log.Printf("monitor: product detection: extract: %v", err)
		return
	}
	if current == nil {
		log.Printf("monitor: product detection: no product data found on page %s", m.URL)
		return
	}

	// Load and persist product state.
	storedJSON := m.storage.GetContent(m.id)
	var stored *ProductState
	if storedJSON != "" {
		stored = &ProductState{}
		if err := json.Unmarshal([]byte(storedJSON), stored); err != nil {
			log.Printf("monitor: product detection: parse stored state: %v", err)
			stored = nil
		}
	}
	stateJSON, _ := json.Marshal(current)
	m.storage.WriteContent(m.id, string(stateJSON))

	if stored == nil {
		log.Printf("monitor: initial product state recorded for %q (inStock=%v price=%.2f)", m.Name, current.InStock, current.Price)
		return
	}

	pd := m.ProductDetection
	var changes []string

	if pd.TrackStock && current.InStock != stored.InStock {
		if current.InStock {
			changes = append(changes, "back in stock")
		} else {
			changes = append(changes, "now out of stock")
		}
	}

	if pd.TrackPrice && current.Price != stored.Price {
		meetsMin := pd.MinPrice == nil || current.Price >= *pd.MinPrice
		meetsMax := pd.MaxPrice == nil || current.Price <= *pd.MaxPrice
		if meetsMin && meetsMax {
			changes = append(changes, fmt.Sprintf("price changed from %.2f to %.2f", stored.Price, current.Price))
		}
	}

	if len(changes) == 0 {
		log.Printf("monitor: no relevant product change for %q, next check in %s", m.Name, m.Interval*time.Minute)
		return
	}

	changeStr := strings.Join(changes, "; ")
	log.Printf("monitor: %q product change: %s", m.Name, changeStr)
	if err := m.notifier.Notify(
		context.Background(),
		fmt.Sprintf("ChangeMonitor: %s – %s", m.Name, changeStr),
		fmt.Sprintf("%s\n\n%s\n\nURL: %s", m.Name, changeStr, m.URL),
	); err != nil {
		log.Printf("monitor: notify: %v", err)
	}
}

// extractProductData scans raw HTML for structured product information.
// It first tries schema.org JSON-LD, then Open Graph / product meta tags.
// Returns nil (no error) when no product data is found on the page.
func extractProductData(body []byte) (*ProductState, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("extract product: parse html: %w", err)
	}

	// --- JSON-LD ---
	var state *ProductState
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return true
		}
		var top any
		if err := json.Unmarshal([]byte(raw), &top); err != nil {
			return true
		}
		// @graph wrapper
		if m, ok := top.(map[string]any); ok {
			if graph, ok := m["@graph"].([]any); ok {
				for _, item := range graph {
					if ps := productStateFromLDNode(item); ps != nil {
						state = ps
						return false
					}
				}
				return true
			}
		}
		// Array of nodes
		if arr, ok := top.([]any); ok {
			for _, item := range arr {
				if ps := productStateFromLDNode(item); ps != nil {
					state = ps
					return false
				}
			}
			return true
		}
		// Single node
		if ps := productStateFromLDNode(top); ps != nil {
			state = ps
			return false
		}
		return true
	})
	if state != nil {
		return state, nil
	}

	// --- Open Graph / product meta tags ---
	var price float64
	var hasPrice, hasStock, inStock bool
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		prop := s.AttrOr("property", "")
		content := s.AttrOr("content", "")
		switch prop {
		case "product:price:amount", "og:price:amount":
			if p, ok := parsePrice(content); ok {
				price = p
				hasPrice = true
			}
		case "product:availability", "og:availability":
			hasStock = true
			inStock = isInStockString(content)
		}
	})
	if hasPrice || hasStock {
		return &ProductState{InStock: inStock, Price: price}, nil
	}

	return nil, nil
}

func productStateFromLDNode(node any) *ProductState {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	typeVal, _ := m["@type"].(string)
	switch typeVal {
	case "Product":
		offersRaw, ok := m["offers"]
		if !ok {
			return nil
		}
		return productStateFromOffers(offersRaw)
	case "Offer", "AggregateOffer":
		return productStateFromOffer(m)
	}
	return nil
}

func productStateFromOffers(offers any) *ProductState {
	switch v := offers.(type) {
	case map[string]any:
		return productStateFromOffer(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if ps := productStateFromOffer(m); ps != nil {
					return ps
				}
			}
		}
	}
	return nil
}

func productStateFromOffer(offer map[string]any) *ProductState {
	var state ProductState
	switch v := offer["price"].(type) {
	case float64:
		state.Price = v
	case string:
		if p, ok := parsePrice(v); ok {
			state.Price = p
		}
	}
	if avail, ok := offer["availability"].(string); ok {
		state.InStock = isInStockString(avail)
	}
	return &state
}

func isInStockString(s string) bool {
	lower := strings.ToLower(s)
	// Explicit out-of-stock terms take priority.
	for _, term := range []string{"outofstock", "out_of_stock", "soldout", "sold_out", "discontinued"} {
		if strings.Contains(lower, term) {
			return false
		}
	}
	for _, term := range []string{"instock", "in_stock", "instoreonly", "onlineonly", "limitedavailability", "preorder", "presale", "available"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func parsePrice(s string) (float64, bool) {
	s = strings.TrimSpace(s)

	// If both separators are present, the last one is the decimal separator.
	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")

	switch {
	case lastComma > lastDot:
		// Danish/German style: 1.299,95 — comma is decimal
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	default:
		// English style: 1,299.95 — dot is decimal
		s = strings.ReplaceAll(s, ",", "")
	}

	// Strip any remaining non-numeric characters (currency symbols etc.)
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' {
			return r
		}
		return -1
	}, s)

	p, err := strconv.ParseFloat(s, 64)
	return p, err == nil
}
