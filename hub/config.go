package hub

import (
	"os"
	"path/filepath"
)

/*
HubConfig holds settings for the Hugging Face Hub client.
*/
type HubConfig struct {
	Endpoint   string
	CacheDir   string
	Token      string
	Offline    bool
	MaxWorkers int
	Xet        HubXetConfig
}

/*
HubXetConfig controls Xet-backed downloads.
*/
type HubXetConfig struct {
	Active bool
}

/*
DefaultHubConfig returns sensible defaults for a Hub client.
*/
func DefaultHubConfig() *HubConfig {
	cacheDir := filepath.Join(".cache", "huggingface", "hub")

	if home, err := os.UserHomeDir(); err == nil {
		cacheDir = filepath.Join(home, ".cache", "huggingface", "hub")
	}

	return &HubConfig{
		Endpoint:   "https://huggingface.co",
		CacheDir:   cacheDir,
		MaxWorkers: 8,
		Xet:        HubXetConfig{Active: true},
	}
}
