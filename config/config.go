package config

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

type Config struct {
	Monitors  monitor.Monitors `json:"monitors"`
	Notifiers NotifiersConfig  `json:"notifiers"`
}

// NotifiersConfig holds the configuration for each supported notifier type.
// Fields are optional; only configured notifiers will be initialized.
type NotifiersConfig struct {
	Pushover *PushoverConfig `json:"pushover,omitempty"`
}

type PushoverConfig struct {
	APIToken string `json:"apiToken"`
	UserKey  string `json:"userKey"`
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

// JSON serializes the config to indented JSON without HTML escaping.
func (c *Config) JSON() ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")
	err := encoder.Encode(c)
	return buffer.Bytes(), err
}
