package hackerone

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pajarori/assetlas/internal/platform"
)

const (
	name           = "hackerone"
	directoryURL   = "https://hackerone.com/directory/programs"
	graphqlURL     = "https://hackerone.com/graphql"
	teamsPageSize  = 100
	scopesPageSize = 100
	batchSize      = 3
	workers        = 16
	csrfBudget     = 64 << 10

	stateSandboxed    = "sandboxed"
	stateSoftLaunched = "soft_launched"
)

var csrfRe = regexp.MustCompile(`<meta\s+name="csrf-token"\s+content="([^"]+)"`)

type Scraper struct {
	deps    platform.Deps
	token   string
	headers map[string]string
}

func New(deps platform.Deps) *Scraper {
	return &Scraper{deps: deps}
}

func (s *Scraper) Name() string { return name }

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type teamsResp struct {
	Data struct {
		Teams struct {
			PageInfo pageInfo `json:"pageInfo"`
			Edges    []struct {
				Node teamNode `json:"node"`
			} `json:"edges"`
		} `json:"teams"`
	} `json:"data"`
	Errors []graphqlError `json:"errors"`
}

type teamNode struct {
	Handle          string `json:"handle"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	OffersBounties  bool   `json:"offers_bounties"`
	SubmissionState string `json:"submission_state"`
	State           string `json:"state"`
}

type pageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

type teamScopeData struct {
	Handle           string `json:"handle"`
	Name             string `json:"name"`
	URL              string `json:"url"`
	OffersBounties   bool   `json:"offers_bounties"`
	StructuredScopes struct {
		PageInfo pageInfo    `json:"pageInfo"`
		Edges    []scopeEdge `json:"edges"`
	} `json:"structured_scopes"`
}

type scopeEdge struct {
	Node scopeNode `json:"node"`
}

type scopeNode struct {
	AssetIdentifier       string  `json:"asset_identifier"`
	AssetType             string  `json:"asset_type"`
	EligibleForSubmission bool    `json:"eligible_for_submission"`
	EligibleForBounty     bool    `json:"eligible_for_bounty"`
	MaxSeverity           string  `json:"max_severity"`
	Instruction           string  `json:"instruction"`
	ArchivedAt            *string `json:"archived_at"`
}

type graphqlError struct {
	Message string   `json:"message"`
	Path    []string `json:"path"`
}

type batchResp struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphqlError             `json:"errors"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]platform.Program, error) {
	if err := s.warmup(ctx); err != nil {
		return nil, fmt.Errorf("warmup: %w", err)
	}
	teams, err := s.listTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	batches := chunkTeams(teams, batchSize)
	log.Info().Str("platform", name).Int("teams", len(teams)).Int("batches", len(batches)).Int("workers", workers).Int("batch_size", batchSize).Msg("found teams")
	return platform.FetchInPoolMulti(ctx, name, batches, workers, s.fetchBatch), nil
}

func (s *Scraper) warmup(ctx context.Context) error {
	body, err := s.deps.Fetcher.GetBody(ctx, directoryURL, nil, csrfBudget)
	if err != nil {
		return err
	}
	m := csrfRe.FindSubmatch(body)
	if m == nil {
		return fmt.Errorf("csrf token not found")
	}
	s.token = string(m[1])
	s.headers = map[string]string{
		"X-CSRF-Token": s.token,
		"X-Auth-Token": s.token,
		"Referer":      directoryURL,
		"Origin":       "https://hackerone.com",
	}
	return nil
}

func (s *Scraper) listTeams(ctx context.Context) ([]teamNode, error) {
	var all []teamNode
	cursor := ""
	for {
		vars := map[string]any{
			"first": teamsPageSize,
			"where": map[string]any{
				"submission_state": map[string]any{"_eq": "open"},
			},
		}
		if cursor != "" {
			vars["after"] = cursor
		}
		req := graphqlRequest{Query: teamsQuery, Variables: vars}
		var resp teamsResp
		if err := s.deps.Fetcher.PostJSONInto(ctx, graphqlURL, req, s.headers, &resp); err != nil {
			return nil, fmt.Errorf("cursor %q: %w", cursor, err)
		}
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("graphql: %s", resp.Errors[0].Message)
		}
		for _, e := range resp.Data.Teams.Edges {
			if e.Node.State == stateSandboxed || e.Node.State == stateSoftLaunched {
				continue
			}
			all = append(all, e.Node)
		}
		if !resp.Data.Teams.PageInfo.HasNextPage {
			break
		}
		cursor = resp.Data.Teams.PageInfo.EndCursor
	}
	return all, nil
}

func chunkTeams(teams []teamNode, size int) [][]teamNode {
	var batches [][]teamNode
	for i := 0; i < len(teams); i += size {
		end := i + size
		if end > len(teams) {
			end = len(teams)
		}
		batches = append(batches, teams[i:end])
	}
	return batches
}

func (s *Scraper) fetchBatch(ctx context.Context, batch []teamNode) ([]platform.Program, error) {
	query, vars := buildBatchQuery(batch)
	var resp batchResp
	if err := s.deps.Fetcher.PostJSONInto(ctx, graphqlURL, graphqlRequest{Query: query, Variables: vars}, s.headers, &resp); err != nil {
		return nil, fmt.Errorf("batch: %w", err)
	}
	for _, gErr := range resp.Errors {
		log.Debug().Str("msg", gErr.Message).Strs("path", gErr.Path).Msg("graphql per-alias error (continuing)")
	}

	out := make([]platform.Program, 0, len(batch))
	for i, t := range batch {
		raw, ok := resp.Data[fmt.Sprintf("t%d", i)]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			log.Debug().Str("handle", t.Handle).Msg("alias missing in batch response")
			continue
		}
		var data teamScopeData
		if err := json.Unmarshal(raw, &data); err != nil {
			log.Warn().Err(err).Str("handle", t.Handle).Msg("decode alias failed")
			continue
		}
		prog := teamToProgram(t, data, data.StructuredScopes.Edges)
		if data.StructuredScopes.PageInfo.HasNextPage {
			full, perr := s.fetchScopesPaginated(ctx, t, scopesPageSize)
			if perr != nil {
				log.Warn().Err(perr).Str("handle", t.Handle).Msg("paginated fallback failed")
			} else {
				prog = full
			}
		}
		out = append(out, prog)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("batch returned 0 programs")
	}
	return out, nil
}

func (s *Scraper) fetchScopesPaginated(ctx context.Context, t teamNode, first int) (platform.Program, error) {
	var scopes []scopeNode
	cursor := ""
	for {
		vars := map[string]any{"handle": t.Handle, "first": first}
		if cursor != "" {
			vars["after"] = cursor
		}
		req := graphqlRequest{Query: singleScopesQuery, Variables: vars}
		var resp singleScopesResp
		if err := s.deps.Fetcher.PostJSONInto(ctx, graphqlURL, req, s.headers, &resp); err != nil {
			return platform.Program{}, fmt.Errorf("scopes: %w", err)
		}
		if len(resp.Errors) > 0 {
			return platform.Program{}, fmt.Errorf("graphql: %s", resp.Errors[0].Message)
		}
		for _, e := range resp.Data.Team.StructuredScopes.Edges {
			if e.Node.ArchivedAt != nil {
				continue
			}
			scopes = append(scopes, e.Node)
		}
		if !resp.Data.Team.StructuredScopes.PageInfo.HasNextPage {
			break
		}
		cursor = resp.Data.Team.StructuredScopes.PageInfo.EndCursor
	}

	edges := make([]scopeEdge, len(scopes))
	for i, sc := range scopes {
		edges[i].Node = sc
	}
	return teamToProgram(t, teamScopeData{Handle: t.Handle, Name: t.Name, URL: t.URL, OffersBounties: t.OffersBounties}, edges), nil
}

type singleScopesResp struct {
	Data struct {
		Team teamScopeData `json:"team"`
	} `json:"data"`
	Errors []graphqlError `json:"errors"`
}

func teamToProgram(t teamNode, data teamScopeData, edges []scopeEdge) platform.Program {
	prog := platform.Program{
		Platform:     name,
		Handle:       t.Handle,
		Name:         platform.FirstNonEmpty(data.Name, t.Name),
		URL:          platform.FirstNonEmpty(data.URL, t.URL),
		OffersBounty: t.OffersBounties || data.OffersBounties,
		FetchedAt:    time.Now().UTC(),
	}
	for _, e := range edges {
		if e.Node.ArchivedAt != nil {
			continue
		}
		identifier := strings.TrimSpace(e.Node.AssetIdentifier)
		if identifier == "" {
			continue
		}
		prog.Scopes = append(prog.Scopes, platform.Scope{
			Identifier:  identifier,
			Type:        platform.DetectAssetType(e.Node.AssetType, identifier),
			Eligible:    e.Node.EligibleForSubmission,
			MaxSeverity: e.Node.MaxSeverity,
			Description: strings.TrimSpace(e.Node.Instruction),
		})
	}
	return prog
}

func buildBatchQuery(teams []teamNode) (string, map[string]any) {
	var sig strings.Builder
	var body strings.Builder
	vars := make(map[string]any, len(teams))
	sig.WriteString("query batch(")
	body.WriteString("{")
	for i, t := range teams {
		if i > 0 {
			sig.WriteString(",")
		}
		fmt.Fprintf(&sig, "$h%d:String!", i)
		fmt.Fprintf(&body, ` t%d: team(handle:$h%d){handle name url offers_bounties structured_scopes(first:%d,archived:false){pageInfo{endCursor hasNextPage} edges{node{asset_identifier asset_type eligible_for_submission eligible_for_bounty max_severity instruction archived_at}}}}`, i, i, scopesPageSize)
		vars[fmt.Sprintf("h%d", i)] = t.Handle
	}
	sig.WriteString(")")
	body.WriteString("}")
	return sig.String() + body.String(), vars
}

const teamsQuery = `query($first:Int!,$after:String,$where:FiltersTeamFilterInput){
  teams(first:$first,after:$after,where:$where){
    pageInfo{endCursor hasNextPage}
    edges{node{handle name url offers_bounties submission_state state}}
  }
}`

const singleScopesQuery = `query($handle:String!,$first:Int!,$after:String){
  team(handle:$handle){
    handle name url offers_bounties
    structured_scopes(first:$first,after:$after,archived:false){
      pageInfo{endCursor hasNextPage}
      edges{node{asset_identifier asset_type eligible_for_submission eligible_for_bounty max_severity instruction archived_at}}
    }
  }
}`
