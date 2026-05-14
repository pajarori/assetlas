package hackenproof

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
)

const (
	name     = "hackenproof"
	host     = "https://hackenproof.com"
	listPath = "/bug-bounty-programs-list"
	workers  = 6
)

type Scraper struct {
	deps    platform.Deps
	headers map[string]string
}

func New(deps platform.Deps) *Scraper {
	hdr := map[string]string{
		"Accept":  "application/json",
		"Referer": host + "/programs",
	}
	if deps.APIKey != "" {
		hdr["hp-partners-bypass"] = deps.APIKey
	}
	return &Scraper{deps: deps, headers: hdr}
}

func (s *Scraper) Name() string { return name }

type listResp struct {
	Items    []listItem `json:"items"`
	NextPage *int       `json:"next_page"`
}

type listItem struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Bounty    bool   `json:"bounty"`
	MaxReward int    `json:"max_reward"`
}

type programDetail struct {
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	Bounty  bool     `json:"bounty"`
	Targets []target `json:"targets"`
}

type target struct {
	Target            string `json:"target"`
	Title             string `json:"title"`
	TargetDescription string `json:"target_description"`
	Criticality       string `json:"criticality"`
	RewardType        string `json:"reward_type"`
	OutOfScope        bool   `json:"out_of_scope"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	if s.deps.APIKey == "" {
		log.Warn().Str("platform", name).Msg("HACKENPROOF_BYPASS not set; skipping (HackenProof public API is gated)")
		return nil, nil
	}
	items, err := s.listPrograms(ctx)
	if err != nil {
		return nil, fmt.Errorf("list programs: %w", err)
	}
	log.Info().Str("platform", name).Int("count", len(items)).Int("workers", workers).Msg("found programs")
	return platform.FetchInPool(ctx, name, items, workers, s.fetchDetail), nil
}

func (s *Scraper) listPrograms(ctx context.Context) ([]listItem, error) {
	var all []listItem
	page := 1
	for {
		url := fmt.Sprintf("%s%s?page=%d", host, listPath, page)
		var resp listResp
		if err := s.deps.Fetcher.GetJSON(ctx, url, s.headers, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Items) == 0 {
			break
		}
		all = append(all, resp.Items...)
		if resp.NextPage == nil {
			break
		}
		page = *resp.NextPage
	}
	return all, nil
}

func (s *Scraper) fetchDetail(ctx context.Context, it listItem) (platform.Program, error) {
	url := fmt.Sprintf("%s%s/%s", host, listPath, it.Slug)
	var detail programDetail
	if err := s.deps.Fetcher.GetJSON(ctx, url, s.headers, &detail); err != nil {
		return platform.Program{}, err
	}
	prog := platform.Program{
		Platform:     name,
		Handle:       detail.Slug,
		Name:         detail.Title,
		URL:          host + "/programs/" + detail.Slug,
		OffersBounty: detail.Bounty,
		FetchedAt:    time.Now().UTC(),
	}
	for _, t := range detail.Targets {
		identifier := strings.TrimSpace(t.Target)
		if identifier == "" {
			continue
		}
		prog.Scopes = append(prog.Scopes, platform.Scope{
			Identifier:  identifier,
			Type:        platform.DetectAssetType(t.Title, identifier),
			Eligible:    !t.OutOfScope,
			MaxSeverity: t.Criticality,
			Description: strings.TrimSpace(t.TargetDescription),
		})
	}
	return prog, nil
}
