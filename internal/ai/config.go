// Package ai is a bounded HTTP adapter for the Meta Model API. It knows wire
// shapes only; intent, obligations and verdicts belong to other packages.
package ai

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.meta.ai/v1"
	DefaultModel   = "muse-spark-1.3-contributor"

	DefaultRequestTimeout   = 45 * time.Second
	DefaultMaxRequestBytes  = 256 << 10
	DefaultMaxResponseBytes = 1 << 20
	MaxContentBytes         = 64 << 10
	MaxToolArgumentBytes    = 16 << 10
	MaxToolCalls            = 8
)

var ErrNotConfigured = errors.New("meta model api is not configured")

// Config holds provider connection settings. APIKey comes from the caller's
// environment; it is never serialized, logged or copied into provenance.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	// HTTPClient may be injected by tests; when nil a no-redirect client is used.
	HTTPClient       *http.Client
	RequestTimeout   time.Duration
	MaxRequestBytes  int
	MaxResponseBytes int64
}

// ConfigFromEnv reads adapter settings explicitly. Callers must invoke it only
// after the operator enabled AI; the offline foundation never calls it.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg := Config{
		APIKey:  getenv("META_MODEL_API_KEY"),
		BaseURL: getenv("PLENILUNE_META_BASE_URL"),
		Model:   getenv("PLENILUNE_META_MODEL"),
	}
	if cfg.APIKey == "" {
		return Config{}, ErrNotConfigured
	}
	return normalizeConfig(cfg)
}

func normalizeConfig(cfg Config) (Config, error) {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		return Config{}, ErrNotConfigured
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, errors.New("invalid Meta API base URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return Config{}, errors.New("Meta API base URL must use https")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	if len(model) > 128 {
		return Config{}, errors.New("model identifier is too long")
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	reqMax := cfg.MaxRequestBytes
	if reqMax <= 0 {
		reqMax = DefaultMaxRequestBytes
	}
	respMax := cfg.MaxResponseBytes
	if respMax <= 0 {
		respMax = DefaultMaxResponseBytes
	}
	return Config{
		APIKey:           key,
		BaseURL:          base,
		Model:            model,
		HTTPClient:       cfg.HTTPClient,
		RequestTimeout:   timeout,
		MaxRequestBytes:  reqMax,
		MaxResponseBytes: respMax,
	}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Endpoint returns the Responses URL for the configured base.
func (c Config) Endpoint() string { return c.BaseURL + "/responses" }

// String is safe to log: it never contains the API key.
func (c Config) String() string {
	return fmt.Sprintf("meta(model=%s, base=%s)", c.Model, c.BaseURL)
}
