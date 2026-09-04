package takeover

import (
	"context"
	"strings"

	"github.com/pajarori/assetlas/internal/tools"
)

type Severity string

const (
	SeverityConfirmed Severity = "confirmed_nxdomain"
	SeverityLikely    Severity = "likely_drift"
	SeverityReview    Severity = "review"
)

type Candidate struct {
	Platform     string   `json:"platform"`
	Handle       string   `json:"handle"`
	Host         string   `json:"host"`
	Severity     Severity `json:"severity"`
	Reason       string   `json:"reason"`
	CNAME        string   `json:"cname"`
	OldCode      int      `json:"old_status"`
	OldTitle     string   `json:"old_title"`
	NewCode      int      `json:"new_status"`
	NewTitle     string   `json:"new_title"`
	NowDNSStatus string   `json:"now_dns_status,omitempty"`
}

func apex(host string) string {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return host
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

func isExternal(host, cname string) bool {
	if cname == "" {
		return false
	}
	return apex(cname) != apex(host)
}

func lastCNAME(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(chain[len(chain)-1], "."))
}

func isErrorLike(code int) bool {
	if code == 0 {
		return true
	}
	return code == 403 || code == 404 || code == 410 || code == 421 || code == 502 || code == 503 || code == 526
}

func Diff(ctx context.Context, old, new []tools.HttpxResult, dnsxOpts tools.DnsxOptions) ([]Candidate, error) {
	oldByHost := make(map[string]tools.HttpxResult, len(old))
	for _, r := range old {
		oldByHost[strings.ToLower(r.Host)] = r
	}
	newByHost := make(map[string]tools.HttpxResult, len(new))
	for _, r := range new {
		newByHost[strings.ToLower(r.Host)] = r
	}

	var out []Candidate
	var recheck []string
	recheckCNAME := make(map[string]string)

	for host, o := range oldByHost {
		cname := lastCNAME(o.CNAME)
		if !isExternal(host, cname) {
			continue
		}
		n, stillAlive := newByHost[host]
		if !stillAlive {
			recheck = append(recheck, host)
			recheckCNAME[host] = cname
			continue
		}
		if lastCNAME(n.CNAME) != cname {
			continue
		}
		switch {
		case isErrorLike(n.StatusCode) && !isErrorLike(o.StatusCode):
			out = append(out, Candidate{
				Host: host, Severity: SeverityLikely, CNAME: cname,
				Reason:  "same external CNAME, status went from healthy to error-like",
				OldCode: o.StatusCode, OldTitle: o.Title,
				NewCode: n.StatusCode, NewTitle: n.Title,
			})
		case o.Title != "" && n.Title != "" && o.Title != n.Title:
			out = append(out, Candidate{
				Host: host, Severity: SeverityReview, CNAME: cname,
				Reason:  "same external CNAME, page title changed",
				OldCode: o.StatusCode, OldTitle: o.Title,
				NewCode: n.StatusCode, NewTitle: n.Title,
			})
		}
	}

	if len(recheck) > 0 {
		dnsxOpts.Hosts = recheck
		if err := tools.RunDnsx(ctx, dnsxOpts, func(r tools.DnsxResult) error {
			host := strings.ToLower(r.Host)
			cname := recheckCNAME[host]
			if lastCNAME(r.CNAME) != cname {
				return nil
			}
			if r.Status == "NXDOMAIN" || r.Status == "SERVFAIL" {
				o := oldByHost[host]
				out = append(out, Candidate{
					Host: host, Severity: SeverityConfirmed, CNAME: cname,
					Reason:  "dangling CNAME still configured, target no longer resolves (" + r.Status + ")",
					OldCode: o.StatusCode, OldTitle: o.Title,
					NowDNSStatus: r.Status,
				})
			}
			return nil
		}); err != nil {
			return out, err
		}
	}

	return out, nil
}
