package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
	"github.com/pajarori/assetlas/internal/store"
	"github.com/pajarori/assetlas/internal/tools"
)

type ProgramMeta struct {
	Platform    string    `json:"platform"`
	Handle      string    `json:"handle"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Wildcards   []string  `json:"wildcards"`
	DirectHosts []string  `json:"direct_hosts"`
	AddedAt     time.Time `json:"added_at"`
	LastSeen    time.Time `json:"last_seen"`
	Archived    bool      `json:"archived"`
}

type Config struct {
	OutputDir     string
	ResolversPath string
	SubfinderArgs tools.SubfinderOptions
	HttpxArgs     tools.HttpxOptions
	NaabuArgs     tools.NaabuOptions
	DnsxArgs      tools.DnsxOptions
	TlsxArgs      tools.TlsxOptions
	WithPorts     bool
	WithDNS       bool
	WithTLS       bool
	TargetWorkers int
}

type Runner struct {
	cfg Config
}

func New(cfg Config) *Runner {
	if cfg.TargetWorkers <= 0 {
		cfg.TargetWorkers = 4
	}
	return &Runner{cfg: cfg}
}

func (r *Runner) Run(ctx context.Context, prog platform.Program) error {
	targets := ExtractTargets(prog)
	if targets.IsEmpty() {
		log.Info().Str("handle", prog.Handle).Str("platform", prog.Platform).Msg("no enumerable targets, skipping")
		return nil
	}
	dir := filepath.Join(r.cfg.OutputDir, prog.Platform, prog.Handle)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := r.writeMeta(dir, prog, targets); err != nil {
		return err
	}

	enumerated, err := r.runSubfinder(ctx, prog.Handle, targets.Wildcards)
	if err != nil {
		log.Warn().Err(err).Str("handle", prog.Handle).Msg("subfinder step failed")
	}
	hosts := mergeHosts(enumerated, targets.DirectHosts)
	if err := writeLines(filepath.Join(dir, "hostnames.txt"), hosts); err != nil {
		return err
	}
	log.Info().Str("handle", prog.Handle).Int("wildcards", len(targets.Wildcards)).Int("direct", len(targets.DirectHosts)).Int("enumerated", len(enumerated)).Int("total_hosts", len(hosts)).Msg("hosts ready")
	if len(hosts) == 0 {
		return nil
	}

	aliveURLs, err := r.runHttpx(ctx, dir, hosts)
	if err != nil {
		log.Warn().Err(err).Str("handle", prog.Handle).Msg("httpx step failed")
	}
	log.Info().Str("handle", prog.Handle).Int("alive", len(aliveURLs)).Msg("httpx done")

	if r.cfg.WithPorts || r.cfg.WithDNS || r.cfg.WithTLS {
		r.runParallelProbes(ctx, dir, hosts, prog.Handle)
	}
	return nil
}

func (r *Runner) runParallelProbes(ctx context.Context, dir string, hosts []string, handle string) {
	var wg sync.WaitGroup
	if r.cfg.WithPorts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runNaabu(ctx, dir, hosts); err != nil {
				log.Warn().Err(err).Str("handle", handle).Msg("naabu step failed")
			}
		}()
	}
	if r.cfg.WithDNS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runDnsx(ctx, dir, hosts); err != nil {
				log.Warn().Err(err).Str("handle", handle).Msg("dnsx step failed")
			}
		}()
	}
	if r.cfg.WithTLS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runTlsx(ctx, dir, tlsTargetsFromHosts(hosts)); err != nil {
				log.Warn().Err(err).Str("handle", handle).Msg("tlsx step failed")
			}
		}()
	}
	wg.Wait()
}

func (r *Runner) runSubfinder(ctx context.Context, handle string, targets []string) ([]string, error) {
	var mu sync.Mutex
	hostsByTarget := make([]string, 0, 1024)
	resolvers := r.cfg.ResolversPath

	workers := r.cfg.TargetWorkers
	if workers > len(targets) {
		workers = len(targets)
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				opts := r.cfg.SubfinderArgs
				opts.Domain = target
				if opts.Resolvers == "" {
					opts.Resolvers = resolvers
				}
				err := tools.RunSubfinder(ctx, opts, func(res tools.SubfinderResult) error {
					if res.Host == "" {
						return nil
					}
					host := strings.ToLower(strings.TrimSpace(res.Host))
					mu.Lock()
					hostsByTarget = append(hostsByTarget, host)
					mu.Unlock()
					return nil
				})
				if err != nil {
					log.Warn().Err(err).Str("handle", handle).Str("target", target).Msg("subfinder target failed")
					errOnce.Do(func() { firstErr = err })
				}
			}
		}()
	}
	for _, t := range targets {
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	return uniqueSortedLower(hostsByTarget), firstErr
}

func streamJSONL[T any](path string, run func(emit func(T) error) error) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	defer w.Flush()
	return run(func(v T) error {
		return writeJSON(w, v)
	})
}

func (r *Runner) runHttpx(ctx context.Context, dir string, hosts []string) ([]string, error) {
	var alive []string
	err := streamJSONL(filepath.Join(dir, "alive.jsonl"), func(emit func(tools.HttpxResult) error) error {
		opts := r.cfg.HttpxArgs
		opts.Hosts = hosts
		return tools.RunHttpx(ctx, opts, func(res tools.HttpxResult) error {
			if err := emit(res); err != nil {
				return err
			}
			if res.URL != "" {
				alive = append(alive, res.URL)
			}
			return nil
		})
	})
	return alive, err
}

func (r *Runner) runNaabu(ctx context.Context, dir string, hosts []string) error {
	return streamJSONL(filepath.Join(dir, "ports.jsonl"), func(emit func(tools.NaabuResult) error) error {
		opts := r.cfg.NaabuArgs
		opts.Hosts = hosts
		return tools.RunNaabu(ctx, opts, emit)
	})
}

func (r *Runner) runDnsx(ctx context.Context, dir string, hosts []string) error {
	return streamJSONL(filepath.Join(dir, "dns.jsonl"), func(emit func(tools.DnsxResult) error) error {
		opts := r.cfg.DnsxArgs
		opts.Hosts = hosts
		if opts.Resolvers == "" {
			opts.Resolvers = r.cfg.ResolversPath
		}
		return tools.RunDnsx(ctx, opts, emit)
	})
}

func (r *Runner) runTlsx(ctx context.Context, dir string, hosts []string) error {
	return streamJSONL(filepath.Join(dir, "tls.jsonl"), func(emit func(tools.TlsxResult) error) error {
		opts := r.cfg.TlsxArgs
		opts.Hosts = hosts
		return tools.RunTlsx(ctx, opts, emit)
	})
}

func mergeHosts(enumerated, direct []string) []string {
	return uniqueSortedLower(append(append([]string(nil), enumerated...), direct...))
}

func (r *Runner) writeMeta(dir string, prog platform.Program, targets Targets) error {
	path := filepath.Join(dir, "meta.json")
	now := time.Now().UTC()
	meta := ProgramMeta{
		Platform:    prog.Platform,
		Handle:      prog.Handle,
		Name:        prog.Name,
		URL:         prog.URL,
		Wildcards:   targets.Wildcards,
		DirectHosts: targets.DirectHosts,
		AddedAt:     now,
		LastSeen:    now,
	}
	if existing, err := os.ReadFile(path); err == nil {
		var prev ProgramMeta
		if err := json.Unmarshal(existing, &prev); err == nil && !prev.AddedAt.IsZero() {
			meta.AddedAt = prev.AddedAt
		}
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return store.WriteIfChanged(path, append(data, '\n'))
}

type Targets struct {
	Wildcards   []string
	DirectHosts []string
}

func (t Targets) IsEmpty() bool {
	return len(t.Wildcards) == 0 && len(t.DirectHosts) == 0
}

func ExtractTargets(prog platform.Program) Targets {
	wcSeen := map[string]struct{}{}
	hostSeen := map[string]struct{}{}
	var t Targets
	for _, sc := range prog.Scopes {
		if !sc.Eligible {
			continue
		}
		ident := strings.TrimSpace(sc.Identifier)
		if ident == "" {
			continue
		}
		switch sc.Type {
		case platform.AssetWildcard:
			base := strings.TrimPrefix(ident, "*.")
			if strings.ContainsAny(base, "* :/") {
				continue
			}
			addUnique(&t.Wildcards, wcSeen, base)
		case platform.AssetURL, platform.AssetAPI:
			host := platform.StripURLPath(platform.NormalizeWildcard(ident))
			if host == "" || strings.ContainsAny(host, "* :") {
				continue
			}
			addUnique(&t.DirectHosts, hostSeen, host)
		}
	}
	sort.Strings(t.Wildcards)
	sort.Strings(t.DirectHosts)
	return t
}

func (t Targets) AllUnique() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(t.Wildcards)+len(t.DirectHosts))
	for _, list := range [][]string{t.Wildcards, t.DirectHosts} {
		for _, s := range list {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func addUnique(out *[]string, seen map[string]struct{}, s string) {
	if _, ok := seen[s]; ok {
		return
	}
	seen[s] = struct{}{}
	*out = append(*out, s)
}

func writeLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, err := w.WriteString(l); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := w.WriteByte('\n'); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return w.Flush()
}

func writeJSON(w *bufio.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

func uniqueSortedLower(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func tlsTargetsFromHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h+":443")
	}
	return out
}
