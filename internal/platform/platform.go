package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/fetcher"
)

type AssetType string

const (
	AssetURL      AssetType = "URL"
	AssetWildcard AssetType = "WILDCARD"
	AssetIOS      AssetType = "IOS"
	AssetAndroid  AssetType = "ANDROID"
	AssetAPI      AssetType = "API"
	AssetSource   AssetType = "SOURCE"
	AssetHardware AssetType = "HARDWARE"
	AssetOther    AssetType = "OTHER"
)

type Scope struct {
	Identifier  string    `json:"identifier"`
	Type        AssetType `json:"type"`
	Eligible    bool      `json:"eligible"`
	MaxSeverity string    `json:"max_severity,omitempty"`
	Description string    `json:"description,omitempty"`
}

type Program struct {
	Platform     string    `json:"platform"`
	Handle       string    `json:"handle"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	OffersBounty bool      `json:"offers_bounty"`
	Managed      bool      `json:"managed,omitempty"`
	Scopes       []Scope   `json:"scopes"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type PlatformInfo struct {
	Key  string
	Name string
}

var AllPlatforms = []PlatformInfo{
	{Key: "hackerone", Name: "HackerOne"},
	{Key: "bugcrowd", Name: "Bugcrowd"},
	{Key: "intigriti", Name: "Intigriti"},
	{Key: "yeswehack", Name: "YesWeHack"},
	{Key: "hackenproof", Name: "HackenProof"},
}

func IsKnownPlatform(key string) bool {
	for _, p := range AllPlatforms {
		if p.Key == key {
			return true
		}
	}
	return false
}

type Scraper interface {
	Name() string
	Scrape(ctx context.Context) ([]Program, error)
}

type Deps struct {
	Fetcher *fetcher.Fetcher
	APIKey  string
}

func NormalizeWildcard(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	return strings.ToLower(s)
}

func StripURLPath(host string) string {
	if i := strings.Index(host, "/"); i != -1 {
		return host[:i]
	}
	return host
}

func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func DetectAssetType(category, identifier string) AssetType {
	c := strings.ToLower(strings.TrimSpace(category))
	id := strings.ToLower(strings.TrimSpace(identifier))

	if strings.Contains(id, "play.google.com/store/apps") || strings.HasSuffix(id, ".apk") {
		return AssetAndroid
	}
	if strings.Contains(id, "apps.apple.com") || strings.Contains(id, "itunes.apple.com") || strings.HasSuffix(id, ".ipa") {
		return AssetIOS
	}

	switch c {
	case "url", "website", "web", "web-application":
		if strings.Contains(id, "*") {
			return AssetWildcard
		}
		return AssetURL
	case "wildcard":
		return AssetWildcard
	case "api":
		return AssetAPI
	case "android", "android-application", "google_play", "google_play_app_id", "android_play_store_app_id":
		return AssetAndroid
	case "ios", "ios-application", "apple_store", "apple_store_app_id", "ios_app_store_app_id", "other_apple":
		return AssetIOS
	case "source_code", "source-code", "source":
		return AssetSource
	case "hardware", "device":
		return AssetHardware
	}

	if strings.Contains(id, "*") {
		return AssetWildcard
	}
	if strings.HasPrefix(id, "com.") {
		return AssetAndroid
	}
	if strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://") {
		return AssetURL
	}
	return AssetOther
}

func SortPrograms(programs []Program) {
	sort.Slice(programs, func(i, j int) bool {
		if programs[i].Platform != programs[j].Platform {
			return programs[i].Platform < programs[j].Platform
		}
		return strings.ToLower(programs[i].Handle) < strings.ToLower(programs[j].Handle)
	})
	for i := range programs {
		if programs[i].Scopes == nil {
			programs[i].Scopes = []Scope{}
		}
		sort.Slice(programs[i].Scopes, func(a, b int) bool {
			return programs[i].Scopes[a].Identifier < programs[i].Scopes[b].Identifier
		})
	}
}

func ErrScraperFailed(platform string, err error) error {
	return fmt.Errorf("scrape %s: %w", platform, err)
}

func FetchInPoolMulti[T any](ctx context.Context, platformName string, items []T, workers int, fetch func(ctx context.Context, item T) ([]Program, error)) []Program {
	if workers <= 0 {
		workers = 8
	}
	tasks := make(chan T)
	results := make(chan []Program, len(items))
	var failures int64
	var done int64
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range tasks {
				select {
				case <-ctx.Done():
					return
				default:
				}
				started := time.Now()
				progs, err := fetch(ctx, item)
				elapsed := time.Since(started)
				idx := atomic.AddInt64(&done, 1)
				if err != nil {
					atomic.AddInt64(&failures, 1)
					log.Warn().Err(err).Str("platform", platformName).Dur("took", elapsed).Int64("progress", idx).Int("total", len(items)).Msg("batch fetch failed")
					continue
				}
				log.Info().Str("platform", platformName).Int("programs", len(progs)).Dur("took", elapsed).Int64("progress", idx).Int("total", len(items)).Msg("batch ok")
				results <- progs
			}
		}()
	}

	go func() {
		for _, item := range items {
			select {
			case <-ctx.Done():
				close(tasks)
				return
			case tasks <- item:
			}
		}
		close(tasks)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	flat := make([]Program, 0)
	for batch := range results {
		flat = append(flat, batch...)
	}
	log.Info().Str("platform", platformName).Int("programs", len(flat)).Int64("failed", failures).Int("total_batches", len(items)).Msg("scrape summary")
	return flat
}

func FetchInPool[T any](ctx context.Context, platformName string, items []T, workers int, fetch func(ctx context.Context, item T) (Program, error)) []Program {
	if workers <= 0 {
		workers = 8
	}
	tasks := make(chan T)
	results := make(chan Program, len(items))
	var failures int64
	var done int64
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range tasks {
				select {
				case <-ctx.Done():
					return
				default:
				}
				started := time.Now()
				prog, err := fetch(ctx, item)
				elapsed := time.Since(started)
				idx := atomic.AddInt64(&done, 1)
				if err != nil {
					atomic.AddInt64(&failures, 1)
					log.Warn().Err(err).Str("platform", platformName).Dur("took", elapsed).Int64("progress", idx).Int("total", len(items)).Msg("program fetch failed")
					continue
				}
				log.Info().Str("platform", platformName).Str("handle", prog.Handle).Int("scopes", len(prog.Scopes)).Dur("took", elapsed).Int64("progress", idx).Int("total", len(items)).Msg("program ok")
				results <- prog
			}
		}()
	}

	go func() {
		for _, item := range items {
			select {
			case <-ctx.Done():
				close(tasks)
				return
			case tasks <- item:
			}
		}
		close(tasks)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	programs := make([]Program, 0, len(items))
	for p := range results {
		programs = append(programs, p)
	}
	log.Info().Str("platform", platformName).Int("ok", len(programs)).Int64("failed", failures).Int("total", len(items)).Msg("scrape summary")
	return programs
}
