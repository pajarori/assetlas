package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	resolversURL      = "https://raw.githubusercontent.com/trickest/resolvers/refs/heads/main/resolvers.txt"
	resolversCacheTTL = 12 * time.Hour
	resolversMaxBytes = 4 << 20
)

func DefaultEnumDir() string {
	return "results/enum"
}

func DefaultResolversCacheDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "pajarori", "invent", "resolvers")
	}
	return filepath.Join(os.TempDir(), "invent-resolvers")
}

func EnsureResolvers(ctx context.Context, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir resolvers cache: %w", err)
	}
	path := filepath.Join(dir, "resolvers.txt")
	if info, err := os.Stat(path); err == nil {
		if time.Since(info.ModTime()) < resolversCacheTTL && info.Size() > 0 {
			log.Debug().Str("file", path).Dur("age", time.Since(info.ModTime())).Msg("resolvers cache hit")
			return path, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolversURL, nil)
	if err != nil {
		return path, fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return path, fmt.Errorf("fetch resolvers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return path, fmt.Errorf("fetch resolvers: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, resolversMaxBytes))
	if err != nil {
		return path, fmt.Errorf("read resolvers: %w", err)
	}
	if len(body) == 0 {
		return path, fmt.Errorf("empty resolvers body")
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return path, fmt.Errorf("cache write: %w", err)
	}
	log.Info().Str("file", path).Int("bytes", len(body)).Msg("resolvers refreshed")
	return path, nil
}
