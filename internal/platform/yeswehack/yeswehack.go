package yeswehack

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
)

const (
	name        = "yeswehack"
	apiBase     = "https://api.yeswehack.com"
	programsURL = apiBase + "/programs"
	perPage     = 25
	workers     = 12
)

type Scraper struct {
	deps platform.Deps
}

func New(deps platform.Deps) *Scraper {
	return &Scraper{deps: deps}
}

func (s *Scraper) Name() string { return name }

type listResp struct {
	Items      []listItem `json:"items"`
	Pagination struct {
		NbResults      int `json:"nb_results"`
		ResultsPerPage int `json:"results_per_page"`
		NbPages        int `json:"nb_pages"`
		Page           int `json:"page"`
	} `json:"pagination"`
}

type listItem struct {
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Public          bool    `json:"public"`
	VDP             bool    `json:"vdp"`
	BountyRewardMin float64 `json:"bounty_reward_min"`
}

type programDetail struct {
	Slug            string      `json:"slug"`
	Title           string      `json:"title"`
	VDP             bool        `json:"vdp"`
	Public          bool        `json:"public"`
	BountyRewardMin float64     `json:"bounty_reward_min"`
	Scopes          []scopeItem `json:"scopes"`
}

type scopeItem struct {
	Scope          string `json:"scope"`
	ScopeType      string `json:"scope_type"`
	Description    string `json:"description"`
	BountyEligible bool   `json:"bounty_eligible"`
	BountyReward   bool   `json:"bounty_reward"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	handles, err := s.listHandles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list programs: %w", err)
	}
	log.Info().Str("platform", name).Int("count", len(handles)).Int("workers", workers).Msg("found program handles")
	return platform.FetchInPool(ctx, name, handles, workers, s.fetchProgram), nil
}

func (s *Scraper) listHandles(ctx context.Context) ([]string, error) {
	var handles []string
	page := 1
	for {
		url := fmt.Sprintf("%s?page=%d", programsURL, page)
		var resp listResp
		if err := s.deps.Fetcher.GetJSON(ctx, url, nil, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Items) == 0 {
			break
		}
		for _, item := range resp.Items {
			if item.Slug == "" {
				continue
			}
			handles = append(handles, item.Slug)
		}
		if resp.Pagination.NbPages > 0 && page >= resp.Pagination.NbPages {
			break
		}
		if len(resp.Items) < perPage {
			break
		}
		page++
	}
	return handles, nil
}

func (s *Scraper) fetchProgram(ctx context.Context, slug string) (platform.Program, error) {
	url := fmt.Sprintf("%s/%s", programsURL, slug)
	var detail programDetail
	if err := s.deps.Fetcher.GetJSON(ctx, url, nil, &detail); err != nil {
		return platform.Program{}, err
	}
	prog := platform.Program{
		Platform:     name,
		Handle:       detail.Slug,
		Name:         detail.Title,
		URL:          fmt.Sprintf("https://yeswehack.com/programs/%s", detail.Slug),
		OffersBounty: !detail.VDP && detail.BountyRewardMin > 0,
		FetchedAt:    time.Now().UTC(),
	}
	for _, sc := range detail.Scopes {
		identifier := strings.TrimSpace(sc.Scope)
		if identifier == "" {
			continue
		}
		prog.Scopes = append(prog.Scopes, platform.Scope{
			Identifier:  identifier,
			Type:        platform.DetectAssetType(sc.ScopeType, identifier),
			Eligible:    sc.BountyEligible || !detail.VDP,
			Description: strings.TrimSpace(sc.Description),
		})
	}
	return prog, nil
}
