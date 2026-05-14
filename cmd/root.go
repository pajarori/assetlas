package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/pajarori/assetlas/internal/fetcher"
	"github.com/pajarori/assetlas/internal/platform"
	"github.com/pajarori/assetlas/internal/platform/bugcrowd"
	"github.com/pajarori/assetlas/internal/platform/hackerone"
	"github.com/pajarori/assetlas/internal/platform/intigriti"
	"github.com/pajarori/assetlas/internal/platform/yeswehack"
	"github.com/pajarori/assetlas/internal/readme"
	"github.com/pajarori/assetlas/internal/runner"
	"github.com/pajarori/assetlas/internal/store"
	"github.com/pajarori/assetlas/internal/tools"
	"github.com/pajarori/assetlas/pkg/config"
)

var version = "0.1.0"

var (
	silent     bool
	verbose    bool
	configPath string
)

func Execute() error {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	rootCmd := &cobra.Command{
		Use:           "assetlas",
		Short:         "Bug bounty asset inventory",
		Long:          "Collect public bug bounty program scopes and run asset enumeration",
		Version:       version,
		Example:       "  assetlas scrape\n  assetlas scrape --platform yeswehack\n  assetlas readme",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	rootCmd.PersistentFlags().BoolVar(&silent, "silent", false, "Run without logs")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose debug logs (per-request timing)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to YAML config")

	rootCmd.AddCommand(scrapeCmd())
	rootCmd.AddCommand(readmeCmd())
	rootCmd.AddCommand(enumCmd())
	rootCmd.AddCommand(statusCmd())

	return rootCmd.Execute()
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show data freshness and per-platform stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyLogLevel()
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return runStatus(cfg)
		},
	}
}

func runStatus(cfg *config.Config) error {
	st := store.New(cfg.ResultsDir)
	fmt.Println("assetlas status")
	fmt.Println("=============")

	idx, err := st.ReadIndex()
	if err != nil {
		log.Warn().Err(err).Msg("index.json not readable; run `assetlas scrape` first")
		idx = &store.Index{Counts: map[string]int{}, Scopes: map[string]int{}}
	}

	for _, p := range platform.AllPlatforms {
		info, statErr := os.Stat(st.ProgramsPath(p.Key))
		age := "never"
		size := int64(0)
		if statErr == nil {
			age = time.Since(info.ModTime()).Round(time.Second).String() + " ago"
			size = info.Size()
		}
		fmt.Printf("  %-12s %4d progs · %5d scopes · %s · %d bytes\n", p.Name, idx.Counts[p.Key], idx.Scopes[p.Key], age, size)
	}
	fmt.Println()
	fmt.Printf("  %-12s %4d progs · %5d scopes\n", "TOTAL", idx.Total, idx.TotalScopes)
	fmt.Println()

	enumDir := st.EnumDir()
	entries, err := os.ReadDir(enumDir)
	if err != nil {
		fmt.Printf("  Enum coverage: no %s/ directory yet\n", enumDir)
		return nil
	}
	enumPlats, enumHandles := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		enumPlats++
		handles, _ := os.ReadDir(filepath.Join(enumDir, e.Name()))
		for _, h := range handles {
			if h.IsDir() {
				enumHandles++
			}
		}
	}
	fmt.Printf("  Enum coverage: %d platforms, %d programs in %s/\n", enumPlats, enumHandles, enumDir)
	return nil
}

func scrapeCmd() *cobra.Command {
	var only []string
	cmd := &cobra.Command{
		Use:   "scrape",
		Short: "Scrape public program scopes from bug bounty platforms",
		Example: "  assetlas scrape\n" +
			"  assetlas scrape --platform yeswehack --platform bugcrowd",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyLogLevel()
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if len(only) > 0 {
				cfg.Platforms = only
				if err := config.Validate(cfg); err != nil {
					return err
				}
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runScrape(ctx, cfg)
		},
	}
	cmd.Flags().StringSliceVarP(&only, "platform", "p", nil, "Limit to specific platforms (repeatable)")
	return cmd
}

func enumCmd() *cobra.Command {
	var handle, platName string
	var withPorts, withDNS, withTLS bool
	var threads, limit, concurrency int
	cmd := &cobra.Command{
		Use:   "enum",
		Short: "Run asset enumeration (subfinder/httpx/naabu/dnsx/tlsx) per program",
		Example: "  assetlas enum\n" +
			"  assetlas enum --handle security --platform hackerone\n" +
			"  assetlas enum --ports --dns --tls",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyLogLevel()
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			resolvers, err := runner.EnsureResolvers(ctx, runner.DefaultResolversCacheDir())
			if err != nil {
				log.Warn().Err(err).Msg("resolvers fetch failed, falling back to tool defaults")
				resolvers = ""
			}
			rcfg := runner.Config{
				OutputDir:     runner.DefaultEnumDir(),
				ResolversPath: resolvers,
				TargetWorkers: 8,
				SubfinderArgs: tools.SubfinderOptions{
					Threads:    threads,
					Timeout:    15,
					RateLimit:  300,
					AllSources: false,
				},
				HttpxArgs: tools.HttpxOptions{
					Threads:   200,
					Timeout:   7,
					RateLimit: 500,
				},
				NaabuArgs: tools.NaabuOptions{
					Threads:   50,
					RateLimit: 2000,
					TopPorts:  "100",
					ScanType:  "c",
				},
				DnsxArgs: tools.DnsxOptions{
					Threads:   200,
					Timeout:   3,
					RateLimit: 500,
				},
				TlsxArgs: tools.TlsxOptions{
					Threads: 200,
					Timeout: 5,
				},
				WithPorts: withPorts,
				WithDNS:   withDNS,
				WithTLS:   withTLS,
			}
			r := runner.New(rcfg)
			programs, err := selectPrograms(cfg, platName, handle)
			if err != nil {
				return err
			}
			if limit > 0 && limit < len(programs) {
				programs = programs[:limit]
			}
			log.Info().Int("programs", len(programs)).Int("concurrency", concurrency).Bool("ports", withPorts).Bool("dns", withDNS).Bool("tls", withTLS).Msg("enum start")
			jobs := make(chan progJob, concurrency)
			var wg sync.WaitGroup
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for job := range jobs {
						log.Info().Int("idx", job.idx).Int("total", len(programs)).Str("platform", job.prog.Platform).Str("handle", job.prog.Handle).Msg("enum program")
						if err := r.Run(ctx, job.prog); err != nil {
							log.Error().Err(err).Str("handle", job.prog.Handle).Msg("enum failed")
						}
					}
				}()
			}
			for i, prog := range programs {
				select {
				case <-ctx.Done():
					close(jobs)
					wg.Wait()
					return ctx.Err()
				case jobs <- progJob{idx: i + 1, prog: prog}:
				}
			}
			close(jobs)
			wg.Wait()
			return nil
		},
	}
	cmd.Flags().StringVar(&handle, "handle", "", "Limit to one program handle")
	cmd.Flags().StringVar(&platName, "platform", "", "Limit to one platform")
	cmd.Flags().BoolVar(&withPorts, "ports", false, "Run naabu port scan")
	cmd.Flags().BoolVar(&withDNS, "dns", false, "Run dnsx DNS records lookup")
	cmd.Flags().BoolVar(&withTLS, "tls", false, "Run tlsx TLS info collection")
	cmd.Flags().IntVar(&threads, "threads", 100, "Threads passed to subfinder/httpx")
	cmd.Flags().IntVar(&limit, "limit", 0, "Process at most N programs (0 = all)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 8, "Programs processed in parallel")
	return cmd
}

type progJob struct {
	idx  int
	prog platform.Program
}

func selectPrograms(cfg *config.Config, platName, handle string) ([]platform.Program, error) {
	st := store.New(cfg.ResultsDir)
	var out []platform.Program
	for _, plat := range cfg.Platforms {
		if platName != "" && plat != platName {
			continue
		}
		programs, err := st.ReadPrograms(plat)
		if err != nil {
			log.Warn().Err(err).Str("platform", plat).Msg("read failed")
			continue
		}
		for _, p := range programs {
			if handle != "" && p.Handle != handle {
				continue
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no programs matched (platform=%q handle=%q); run `assetlas scrape` first", platName, handle)
	}
	return out, nil
}

func readmeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "readme",
		Short: "Regenerate README from data/",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyLogLevel()
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return runReadme(cfg)
		},
	}
}

func applyLogLevel() {
	switch {
	case verbose:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case silent:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
}

func runScrape(ctx context.Context, cfg *config.Config) error {
	f := fetcher.New(
		fetcher.WithUserAgent(cfg.UserAgent),
		fetcher.WithTimeout(cfg.RequestTimeout),
		fetcher.WithMaxRetries(cfg.MaxRetries),
		fetcher.WithHostConfig(fetcher.HostConfig{
			InitialConcurrency: 8,
			MinConcurrency:     1,
			MaxConcurrency:     24,
			GrowAfterSuccess:   8,
			MinInterval:        cfg.RequestDelay,
		}),
	)

	scrapers := buildScrapers(cfg, f)
	if len(scrapers) == 0 {
		return fmt.Errorf("no scrapers selected")
	}

	results := make(map[string][]platform.Program, len(scrapers))
	errs := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(scrapers))

	for _, sc := range scrapers {
		go func(sc platform.Scraper) {
			defer wg.Done()
			start := time.Now()
			log.Info().Str("platform", sc.Name()).Msg("scraping")
			programs, err := sc.Scrape(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[sc.Name()] = err
				log.Error().Err(err).Str("platform", sc.Name()).Msg("scrape failed")
				return
			}
			results[sc.Name()] = programs
			log.Info().Str("platform", sc.Name()).Int("programs", len(programs)).Dur("took", time.Since(start)).Msg("scrape done")
		}(sc)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	st := store.New(cfg.ResultsDir)
	for plat, programs := range results {
		if err := st.WritePrograms(plat, programs); err != nil {
			return fmt.Errorf("write %s: %w", plat, err)
		}
	}
	if err := st.WriteAggregates(results); err != nil {
		return fmt.Errorf("write aggregates: %w", err)
	}
	if err := st.WriteIndex(results); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	if err := readme.Generate("README.md", results, version); err != nil {
		log.Warn().Err(err).Msg("readme regeneration failed")
	}

	if len(errs) > 0 {
		names := make([]string, 0, len(errs))
		for k := range errs {
			names = append(names, k)
		}
		sort.Strings(names)
		return fmt.Errorf("partial failure: %s", strings.Join(names, ", "))
	}
	return nil
}

func runReadme(cfg *config.Config) error {
	st := store.New(cfg.ResultsDir)
	all := map[string][]platform.Program{}
	for _, plat := range cfg.Platforms {
		programs, err := st.ReadPrograms(plat)
		if err != nil {
			log.Warn().Err(err).Str("platform", plat).Msg("read failed")
			continue
		}
		all[plat] = programs
	}
	if err := st.WriteAggregates(all); err != nil {
		log.Warn().Err(err).Msg("aggregate regen failed")
	}
	if err := st.WriteIndex(all); err != nil {
		log.Warn().Err(err).Msg("index regen failed")
	}
	return readme.Generate("README.md", all, version)
}

func buildScrapers(cfg *config.Config, f *fetcher.Fetcher) []platform.Scraper {
	deps := platform.Deps{Fetcher: f}
	scrapers := make([]platform.Scraper, 0, len(cfg.Platforms))
	for _, plat := range cfg.Platforms {
		switch plat {
		case config.PlatformHackerOne:
			scrapers = append(scrapers, hackerone.New(deps))
		case config.PlatformBugcrowd:
			scrapers = append(scrapers, bugcrowd.New(deps))
		case config.PlatformIntigriti:
			scrapers = append(scrapers, intigriti.New(deps))
		case config.PlatformYesWeHack:
			scrapers = append(scrapers, yeswehack.New(deps))
		}
	}
	return scrapers
}
