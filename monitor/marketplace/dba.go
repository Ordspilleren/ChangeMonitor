package marketplace

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/PuerkitoBio/goquery"
)

type dbaScraper struct{}

func (s *dbaScraper) RequiresChrome() bool { return false }

func (s *dbaScraper) Fetch(client monitor.MonitorClient, url string, headers http.Header) ([]Listing, error) {
	body, err := client.GetContent(url, headers)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}
	defer body.Close()
	return s.extractFromHTML(body)
}

func (s *dbaScraper) extractFromHTML(r io.Reader) ([]Listing, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parsing dba.dk HTML: %w", err)
	}

	seen := make(map[string]bool)
	var listings []Listing

	doc.Find(`section[aria-labelledby="results-heading"] article.sf-search-ad`).Each(func(_ int, article *goquery.Selection) {
		link := article.Find("a.sf-search-ad-link").First()
		id, _ := link.Attr("id")
		if id == "" || seen[id] {
			return
		}
		seen[id] = true

		href, _ := link.Attr("href")
		link.Find(`span[aria-hidden="true"]`).Remove()
		title := strings.TrimSpace(link.Text())
		priceText := strings.TrimSpace(article.Find("div.whitespace-nowrap span").First().Text())
		imageURL, _ := article.Find("img").First().Attr("src")

		priceStr, priceVal := parseDBAPrice(priceText)
		listings = append(listings, Listing{
			ID:       id,
			URL:      href,
			Title:    title,
			Price:    priceStr,
			PriceVal: priceVal,
			ImageURL: imageURL,
		})
	})

	return listings, nil
}

var dbaPricePattern = regexp.MustCompile(`([\d.,]+)\s*kr\.?`)

func parseDBAPrice(raw string) (priceStr string, priceVal float64) {
	m := dbaPricePattern.FindStringSubmatch(raw)
	if m == nil {
		return raw, 0
	}
	priceStr = m[0]
	priceVal, _ = monitor.ParsePrice(m[1])
	return
}
