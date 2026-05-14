package bugcrowd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
)

const (
	name                 = "bugcrowd"
	host                 = "https://bugcrowd.com"
	engagementsURL       = host + "/engagements.json"
	workers              = 16
	changelogStateLatest = "Latest"
)

var jsonHdr = map[string]string{
	"Accept":           "application/json",
	"X-Requested-With": "XMLHttpRequest",
	"Referer":          host + "/programs",
}

type Scraper struct {
	deps platform.Deps
}

func New(deps platform.Deps) *Scraper {
	return &Scraper{deps: deps}
}

func (s *Scraper) Name() string { return name }

type engagementsResp struct {
	Engagements    []engagement `json:"engagements"`
	PaginationMeta struct {
		Limit      int `json:"limit"`
		TotalCount int `json:"totalCount"`
	} `json:"paginationMeta"`
}

type engagement struct {
	Name         string `json:"name"`
	BriefURL     string `json:"briefUrl"`
	ProgramURL   string `json:"programUrl"`
	AccessStatus string `json:"accessStatus"`
	Category     string `json:"category"`
	MaxRewards   *int   `json:"maxRewards"`
}

type changelogList struct {
	Changelogs []struct {
		ChangelogShowURL string `json:"changelogShowUrl"`
		ChangelogState   string `json:"changelogState"`
	} `json:"changelogs"`
}

type briefResp struct {
	Data struct {
		Brief struct {
			Name string `json:"name"`
		} `json:"brief"`
		Scope []scopeGroup `json:"scope"`
	} `json:"data"`
}

type scopeGroup struct {
	Name        string   `json:"name"`
	InScope     bool     `json:"inScope"`
	Description string   `json:"description"`
	Targets     []target `json:"targets"`
}

type target struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	IPAddress   string `json:"ipAddress"`
	Description string `json:"description"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	engs, err := s.listEngagements(ctx)
	if err != nil {
		return nil, fmt.Errorf("list engagements: %w", err)
	}
	log.Info().Str("platform", name).Int("count", len(engs)).Int("workers", workers).Msg("found engagements")
	return platform.FetchInPool(ctx, name, engs, workers, s.fetchProgram), nil
}

func (s *Scraper) listEngagements(ctx context.Context) ([]engagement, error) {
	var all []engagement
	page := 1
	for {
		url := fmt.Sprintf("%s?category=bug_bounty&sort_by=promoted&sort_direction=desc&page=%d", engagementsURL, page)
		var resp engagementsResp
		if err := s.deps.Fetcher.GetJSON(ctx, url, jsonHdr, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Engagements) == 0 {
			break
		}
		all = append(all, resp.Engagements...)
		if resp.PaginationMeta.Limit > 0 && len(resp.Engagements) < resp.PaginationMeta.Limit {
			break
		}
		if resp.PaginationMeta.TotalCount > 0 && len(all) >= resp.PaginationMeta.TotalCount {
			break
		}
		page++
	}
	return all, nil
}

func (s *Scraper) fetchProgram(ctx context.Context, e engagement) (platform.Program, error) {
	handle := strings.TrimPrefix(e.ProgramURL, "/")
	if handle == "" {
		handle = strings.TrimPrefix(e.BriefURL, "/engagements/")
	}
	prog := platform.Program{
		Platform:     name,
		Handle:       handle,
		Name:         e.Name,
		URL:          host + e.BriefURL,
		OffersBounty: e.MaxRewards != nil && *e.MaxRewards > 0,
		Managed:      true,
		FetchedAt:    time.Now().UTC(),
	}

	briefPath, err := s.discoverBriefPath(ctx, e.BriefURL)
	if err != nil {
		log.Warn().Err(err).Str("handle", handle).Msg("brief discovery failed, emitting metadata-only program")
		return prog, nil
	}

	var brief briefResp
	if err := s.deps.Fetcher.GetJSON(ctx, host+briefPath+".json", jsonHdr, &brief); err != nil {
		log.Warn().Err(err).Str("handle", handle).Msg("brief json fetch failed, emitting metadata-only program")
		return prog, nil
	}

	prog.Name = platform.FirstNonEmpty(brief.Data.Brief.Name, e.Name)
	for _, g := range brief.Data.Scope {
		for _, t := range g.Targets {
			identifier := platform.FirstNonEmpty(t.URI, t.Name, t.IPAddress)
			if identifier == "" {
				continue
			}
			prog.Scopes = append(prog.Scopes, platform.Scope{
				Identifier:  identifier,
				Type:        platform.DetectAssetType(t.Category, identifier),
				Eligible:    g.InScope,
				Description: strings.TrimSpace(t.Description),
			})
		}
	}
	return prog, nil
}

func (s *Scraper) discoverBriefPath(ctx context.Context, briefURL string) (string, error) {
	var cl changelogList
	if err := s.deps.Fetcher.GetJSON(ctx, host+briefURL+"/changelog.json", jsonHdr, &cl); err != nil {
		return "", err
	}
	for _, c := range cl.Changelogs {
		if c.ChangelogState == changelogStateLatest && c.ChangelogShowURL != "" {
			return c.ChangelogShowURL, nil
		}
	}
	if len(cl.Changelogs) > 0 && cl.Changelogs[0].ChangelogShowURL != "" {
		return cl.Changelogs[0].ChangelogShowURL, nil
	}
	return "", fmt.Errorf("no changelog entries")
}
