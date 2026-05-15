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
	name          = "hackenproof"
	dashboardHost = "https://dashboard.hackenproof.com"
	listingPath   = "/api/internal/user/opportunities"
	detailPath    = "/api/internal/user/opportunities/"
	publicHost    = "https://hackenproof.com"
	perPage       = 100
	workers       = 8
)

type Scraper struct {
	deps    platform.Deps
	headers map[string]string
}

func New(deps platform.Deps) *Scraper {
	hdr := map[string]string{
		"Accept":  "application/json",
		"Origin":  dashboardHost,
		"Referer": dashboardHost + "/programs",
	}
	if deps.APIKey != "" {
		hdr["Authorization"] = "Bearer " + deps.APIKey
	}
	return &Scraper{deps: deps, headers: hdr}
}

func (s *Scraper) Name() string { return name }

type listResp struct {
	Programs   []listItem `json:"programs"`
	TotalItems int        `json:"total_items"`
	NextPage   *int       `json:"next_page"`
}

type listItem struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	State string `json:"state"`
}

type detailResp struct {
	Slug      string      `json:"slug"`
	Title     string      `json:"title"`
	State     string      `json:"state"`
	MaxBounty interface{} `json:"max_bounty"`
	MinBounty interface{} `json:"min_bounty"`
	Private   bool        `json:"private"`
	Scopes    []scopeItem `json:"scopes"`
	Company   struct {
		Slug string `json:"slug"`
		Name string `json:"company_name"`
	} `json:"company"`
}

type scopeItem struct {
	Target            string `json:"target"`
	Title             string `json:"title"`
	Criticality       string `json:"criticality"`
	OutOfScope        bool   `json:"out_of_scope"`
	TargetDescription string `json:"target_description"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	if s.deps.APIKey == "" {
		log.Warn().Str("platform", name).Msg("HACKENPROOF_BYPASS not set; skipping (HackenProof dashboard API requires Bearer JWT)")
		return nil, nil
	}
	items, err := s.listOpportunities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list opportunities: %w", err)
	}
	log.Info().Str("platform", name).Int("count", len(items)).Int("workers", workers).Msg("found opportunities")
	return platform.FetchInPool(ctx, name, items, workers, s.fetchDetail), nil
}

func (s *Scraper) listOpportunities(ctx context.Context) ([]listItem, error) {
	var all []listItem
	page := 1
	for {
		url := fmt.Sprintf("%s%s?page=%d&per_page=%d&not_audits=true&order_by%%5Bpublished_date%%5D=desc",
			dashboardHost, listingPath, page, perPage)
		var resp listResp
		if err := s.deps.Fetcher.GetJSON(ctx, url, s.headers, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Programs) == 0 {
			break
		}
		for _, it := range resp.Programs {
			if it.Slug == "" {
				continue
			}
			all = append(all, it)
		}
		if resp.NextPage == nil {
			break
		}
		page = *resp.NextPage
	}
	return all, nil
}

func (s *Scraper) fetchDetail(ctx context.Context, it listItem) (platform.Program, error) {
	url := dashboardHost + detailPath + it.Slug
	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": s.headers["Authorization"],
		"Origin":        dashboardHost,
		"Referer":       dashboardHost + "/programs/" + it.Slug,
	}
	var detail detailResp
	if err := s.deps.Fetcher.GetJSON(ctx, url, headers, &detail); err != nil {
		return platform.Program{}, err
	}
	prog := platform.Program{
		Platform:     name,
		Handle:       detail.Slug,
		Name:         platform.FirstNonEmpty(detail.Title, it.Title),
		URL:          publicHost + "/programs/" + detail.Slug,
		OffersBounty: bountyOffered(detail.MaxBounty),
		FetchedAt:    time.Now().UTC(),
	}
	for _, sc := range detail.Scopes {
		identifier := strings.TrimSpace(sc.Target)
		if identifier == "" {
			continue
		}
		prog.Scopes = append(prog.Scopes, platform.Scope{
			Identifier:  identifier,
			Type:        platform.DetectAssetType(sc.Title, identifier),
			Eligible:    !sc.OutOfScope,
			MaxSeverity: sc.Criticality,
			Description: strings.TrimSpace(sc.TargetDescription),
		})
	}
	return prog, nil
}

func bountyOffered(v interface{}) bool {
	switch x := v.(type) {
	case float64:
		return x > 0
	case int:
		return x > 0
	case string:
		return x != "" && x != "0"
	}
	return false
}
