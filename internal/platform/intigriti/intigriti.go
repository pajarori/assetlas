package intigriti

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
)

var (
	algoliaHdr = map[string]string{
		"X-Algolia-Application-Id": algoliaAppID,
		"X-Algolia-API-Key":        algoliaAPIKey,
	}
	jsonHdr = map[string]string{"Accept": "application/json"}
)

const (
	name                  = "intigriti"
	algoliaAppID          = "AAZUKSYAR4"
	algoliaAPIKey         = "70d8a3400477311f27ce002ec953aeb0"
	algoliaIndex          = "programs_prod"
	algoliaURL            = "https://aazuksyar4-dsn.algolia.net/1/indexes/*/queries"
	publicProgramBase     = "https://app.intigriti.com/api/core/public/programs"
	hitsPerPage           = 1000
	workers               = 12
	statusOpen            = 3
	confidentialityPublic = 4
	tierOutOfScope        = "Out of scope"
)

type Scraper struct {
	deps platform.Deps
}

func New(deps platform.Deps) *Scraper {
	return &Scraper{deps: deps}
}

func (s *Scraper) Name() string { return name }

type algoliaRequest struct {
	Requests []algoliaSubRequest `json:"requests"`
}

type algoliaSubRequest struct {
	IndexName string `json:"indexName"`
	Params    string `json:"params"`
}

type algoliaResponse struct {
	Results []struct {
		Hits    []algoliaHit `json:"hits"`
		NbPages int          `json:"nbPages"`
	} `json:"results"`
}

type algoliaHit struct {
	CompanyHandle        string `json:"companyHandle"`
	Handle               string `json:"handle"`
	Name                 string `json:"name"`
	Status               int    `json:"status"`
	ConfidentialityLevel int    `json:"confidentialityLevel"`
	TacRequired          bool   `json:"tacRequired"`
	TwoFactorRequired    bool   `json:"twoFactorRequired"`
	MaxBounty            bounty `json:"maxBounty"`
}

type bounty struct {
	Value    int    `json:"value"`
	Currency string `json:"currency"`
}

type programDetail struct {
	Handle           string             `json:"handle"`
	CompanyHandle    string             `json:"companyHandle"`
	Name             string             `json:"name"`
	AssetsCollection []assetsCollection `json:"assetsCollection"`
}

type assetsCollection struct {
	CreatedAt int64 `json:"createdAt"`
	Content   struct {
		AssetsAndGroups []assetOrGroup `json:"assetsAndGroups"`
	} `json:"content"`
}

type assetOrGroup struct {
	TypeID       int     `json:"typeId"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	BountyTierID int     `json:"bountyTierId"`
	Assets       []asset `json:"assets"`
}

type asset struct {
	TypeID       int    `json:"typeId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	BountyTierID int    `json:"bountyTierId"`
}

var (
	intigritiTypes = []platform.AssetType{
		platform.AssetOther,
		platform.AssetURL,
		platform.AssetAndroid,
		platform.AssetIOS,
		platform.AssetOther,
		platform.AssetHardware,
		platform.AssetOther,
		platform.AssetWildcard,
	}
	intigritiTiers = []string{"", "No Bounty", "Tier 3", "Tier 2", "Tier 1", "Out of scope"}
)

func intigritiType(typeID int) platform.AssetType {
	if typeID >= 0 && typeID < len(intigritiTypes) {
		return intigritiTypes[typeID]
	}
	return platform.AssetOther
}

func intigritiTier(tierID int) string {
	if tierID >= 0 && tierID < len(intigritiTiers) {
		return intigritiTiers[tierID]
	}
	return ""
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	hits, err := s.listHits(ctx)
	if err != nil {
		return nil, fmt.Errorf("algolia list: %w", err)
	}
	publicHits := make([]algoliaHit, 0, len(hits))
	for _, h := range hits {
		if h.Status != statusOpen || h.ConfidentialityLevel != confidentialityPublic || h.TacRequired || h.TwoFactorRequired {
			continue
		}
		publicHits = append(publicHits, h)
	}
	log.Info().Str("platform", name).Int("total", len(hits)).Int("public", len(publicHits)).Int("workers", workers).Msg("found hits")
	return platform.FetchInPool(ctx, name, publicHits, workers, s.fetchDetail), nil
}

func (s *Scraper) listHits(ctx context.Context) ([]algoliaHit, error) {
	var hits []algoliaHit
	page := 0
	for {
		params := fmt.Sprintf("hitsPerPage=%d&page=%d&query=", hitsPerPage, page)
		body := algoliaRequest{
			Requests: []algoliaSubRequest{
				{IndexName: algoliaIndex, Params: params},
			},
		}
		var resp algoliaResponse
		if err := s.deps.Fetcher.PostJSONInto(ctx, algoliaURL, body, algoliaHdr, &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Results) == 0 || len(resp.Results[0].Hits) == 0 {
			break
		}
		hits = append(hits, resp.Results[0].Hits...)
		if page+1 >= resp.Results[0].NbPages {
			break
		}
		if len(resp.Results[0].Hits) < hitsPerPage {
			break
		}
		page++
	}
	return hits, nil
}

func (s *Scraper) fetchDetail(ctx context.Context, h algoliaHit) (platform.Program, error) {
	prog := platform.Program{
		Platform:     name,
		Handle:       fmt.Sprintf("%s/%s", h.CompanyHandle, h.Handle),
		Name:         h.Name,
		URL:          fmt.Sprintf("https://www.intigriti.com/programs/%s/%s/detail", h.CompanyHandle, h.Handle),
		OffersBounty: h.MaxBounty.Value > 0,
		FetchedAt:    time.Now().UTC(),
	}

	url := fmt.Sprintf("%s/%s/%s", publicProgramBase, h.CompanyHandle, h.Handle)
	resp, err := s.deps.Fetcher.Get(ctx, url, jsonHdr)
	if err != nil {
		return prog, fmt.Errorf("detail fetch: %w", err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		log.Debug().Str("handle", prog.Handle).Str("content_type", ct).Msg("detail unavailable (non-JSON response)")
		return prog, nil
	}
	var detail programDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return prog, fmt.Errorf("parse detail: %w", err)
	}
	if detail.Name != "" {
		prog.Name = detail.Name
	}

	latest := latestAssetsCollection(detail.AssetsCollection)
	for _, item := range latest {
		assets := []asset{}
		if len(item.Assets) > 0 {
			assets = item.Assets
		} else {
			assets = []asset{{TypeID: item.TypeID, Name: item.Name, Description: item.Description, BountyTierID: item.BountyTierID}}
		}
		for _, a := range assets {
			identifier := strings.TrimSpace(a.Name)
			if identifier == "" {
				continue
			}
			tier := intigritiTier(a.BountyTierID)
			prog.Scopes = append(prog.Scopes, platform.Scope{
				Identifier:  identifier,
				Type:        intigritiType(a.TypeID),
				Eligible:    !strings.EqualFold(tier, tierOutOfScope),
				MaxSeverity: tier,
				Description: strings.TrimSpace(a.Description),
			})
		}
	}
	return prog, nil
}

func latestAssetsCollection(cols []assetsCollection) []assetOrGroup {
	if len(cols) == 0 {
		return nil
	}
	latest := cols[0]
	for _, c := range cols[1:] {
		if c.CreatedAt > latest.CreatedAt {
			latest = c
		}
	}
	return latest.Content.AssetsAndGroups
}
