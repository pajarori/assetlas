package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pajarori/assetlas/internal/platform"
)

type Config struct {
	ResultsDir     string            `yaml:"results_dir"`
	UserAgent      string            `yaml:"user_agent"`
	RequestDelay   time.Duration     `yaml:"request_delay"`
	RequestTimeout time.Duration     `yaml:"request_timeout"`
	MaxRetries     int               `yaml:"max_retries"`
	Platforms      []string          `yaml:"platforms"`
	APIKeys        map[string]string `yaml:"api_keys"`
}

const (
	PlatformHackerOne   = "hackerone"
	PlatformBugcrowd    = "bugcrowd"
	PlatformIntigriti   = "intigriti"
	PlatformYesWeHack   = "yeswehack"
	PlatformHackenProof = "hackenproof"
)

func Default() *Config {
	platforms := make([]string, 0, len(platform.AllPlatforms))
	for _, p := range platform.AllPlatforms {
		platforms = append(platforms, p.Key)
	}
	return &Config{
		ResultsDir:     "results",
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36",
		RequestDelay:   200 * time.Millisecond,
		RequestTimeout: 30 * time.Second,
		MaxRetries:     3,
		Platforms:      platforms,
		APIKeys:        map[string]string{},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		} else if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	if key := os.Getenv("HACKENPROOF_BYPASS"); key != "" {
		cfg.APIKeys[PlatformHackenProof] = key
	}
	return cfg, Validate(cfg)
}

func Validate(cfg *Config) error {
	if cfg.ResultsDir == "" {
		return fmt.Errorf("results_dir is required")
	}
	if cfg.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}
	for _, p := range cfg.Platforms {
		if !platform.IsKnownPlatform(p) {
			return fmt.Errorf("unknown platform: %s", p)
		}
	}
	return nil
}
