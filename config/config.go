package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/Ordspilleren/ChangeMonitor/monitor/generic"
	"github.com/Ordspilleren/ChangeMonitor/monitor/marketplace"
	"github.com/Ordspilleren/ChangeMonitor/monitor/product"
)

type Config struct {
	Monitors  []MonitorConfig `json:"monitors"`
	Notifiers NotifiersConfig `json:"notifiers"`
}

// MonitorConfig describes the JSON configuration for a monitor.
// Runtime monitors are created via ToRuntime.
type MonitorConfig struct {
	Name        string      `json:"name"`
	URL         string      `json:"url"`
	HTTPHeaders http.Header `json:"httpHeaders,omitempty"`
	UseChrome   bool        `json:"useChrome"`
	Interval    int64       `json:"interval"`

	Generic     *GenericFeatureConfig     `json:"generic,omitempty"`
	Product     *ProductFeatureConfig     `json:"product,omitempty"`
	Marketplace *MarketplaceFeatureConfig `json:"marketplace,omitempty"`
}

type SelectorConfig struct {
	Type  string   `json:"type,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

type FiltersConfig struct {
	Contains    []string `json:"contains,omitempty"`
	NotContains []string `json:"notContains,omitempty"`
}

type GenericFeatureConfig struct {
	Selector    SelectorConfig `json:"selector,omitempty"`
	Filters     *FiltersConfig `json:"filters,omitempty"`
	IgnoreEmpty bool           `json:"ignoreEmpty,omitempty"`
}

type ProductFeatureConfig struct {
	TrackStock bool     `json:"trackStock,omitempty"`
	TrackPrice bool     `json:"trackPrice,omitempty"`
	MinPrice   *float64 `json:"minPrice,omitempty"`
	MaxPrice   *float64 `json:"maxPrice,omitempty"`
}

type MarketplaceFeatureConfig struct {
	// Selector is a CSS selector that matches one element per listing on the page.
	Selector      string   `json:"selector"`
	LinkSelector  string   `json:"linkSelector,omitempty"`
	TitleSelector string   `json:"titleSelector,omitempty"`
	PriceSelector string   `json:"priceSelector,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	MaxPrice      float64  `json:"maxPrice,omitempty"`
}

// NotifiersConfig holds the configuration for each supported notifier type.
// Fields are optional; only configured notifiers will be initialized.
type NotifiersConfig struct {
	Pushover *PushoverConfig `json:"pushover,omitempty"`
	Logger   *LoggerConfig   `json:"logger,omitempty"`
}

type PushoverConfig struct {
	APIToken string `json:"apiToken"`
	UserKey  string `json:"userKey"`
}

type LoggerConfig struct {
	Enabled bool `json:"enabled"`
}

// Load reads and parses a JSON config file.
func Load(filename string) (*Config, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RuntimeMonitors maps configured monitors into runtime monitors.
func (c *Config) RuntimeMonitors() (monitor.Monitors, error) {
	runtime := make(monitor.Monitors, 0, len(c.Monitors))
	for i := range c.Monitors {
		m, err := c.Monitors[i].ToRuntime()
		if err != nil {
			return nil, fmt.Errorf("monitor %q: %w", c.Monitors[i].Name, err)
		}
		runtime = append(runtime, m)
	}
	return runtime, nil
}

// ToRuntime converts JSON config into a runtime monitor.
func (mc *MonitorConfig) ToRuntime() (monitor.Monitor, error) {
	m := monitor.Monitor{
		Name:        mc.Name,
		URL:         mc.URL,
		HTTPHeaders: mc.HTTPHeaders,
		UseChrome:   mc.UseChrome,
		Interval:    time.Duration(mc.Interval),
	}

	featureCount := 0
	if mc.Generic != nil {
		featureCount++
	}
	if mc.Product != nil {
		featureCount++
	}
	if mc.Marketplace != nil {
		featureCount++
	}
	if featureCount > 1 {
		return monitor.Monitor{}, fmt.Errorf("only one detection feature can be configured")
	}

	if mc.Marketplace != nil {
		m.Feature = &marketplace.MarketplaceFeature{
			Selector:      mc.Marketplace.Selector,
			LinkSelector:  mc.Marketplace.LinkSelector,
			TitleSelector: mc.Marketplace.TitleSelector,
			PriceSelector: mc.Marketplace.PriceSelector,
			Keywords:      mc.Marketplace.Keywords,
			MaxPrice:      mc.Marketplace.MaxPrice,
		}
		return m, nil
	}

	if mc.Product != nil {
		m.Feature = &product.ProductFeature{
			Detection: product.ProductDetection{
				TrackStock: mc.Product.TrackStock,
				TrackPrice: mc.Product.TrackPrice,
				MinPrice:   mc.Product.MinPrice,
				MaxPrice:   mc.Product.MaxPrice,
			},
		}
		return m, nil
	}

	if mc.Generic != nil {
		var filters *generic.Filters
		if mc.Generic.Filters != nil {
			filters = &generic.Filters{
				Contains:    mc.Generic.Filters.Contains,
				NotContains: mc.Generic.Filters.NotContains,
			}
		}
		m.Feature = &generic.GenericFeature{
			Selector: generic.Selector{
				Type:  mc.Generic.Selector.Type,
				Paths: mc.Generic.Selector.Paths,
			},
			Filters:     filters,
			IgnoreEmpty: mc.Generic.IgnoreEmpty,
		}
		return m, nil
	}

	return monitor.Monitor{}, fmt.Errorf("no detection feature configured")
}

// JSON serializes the config to indented JSON without HTML escaping.
func (c *Config) JSON() ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")
	err := encoder.Encode(c)
	return buffer.Bytes(), err
}
