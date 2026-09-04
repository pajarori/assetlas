package takeover

import (
	"context"
	"testing"

	"github.com/pajarori/assetlas/internal/tools"
)

func TestDiffStatusDrift(t *testing.T) {
	old := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Welcome", CNAME: []string{"foo.example.com.herokudns.com"}}}
	new := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 404, Title: "No such app", CNAME: []string{"foo.example.com.herokudns.com"}}}
	c, err := Diff(context.Background(), old, new, tools.DnsxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Severity != SeverityLikely {
		t.Fatalf("want 1 likely_drift finding, got %+v", c)
	}
}

func TestDiffTitleOnlyDrift(t *testing.T) {
	old := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Welcome", CNAME: []string{"foo.example.com.herokudns.com"}}}
	new := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Something else", CNAME: []string{"foo.example.com.herokudns.com"}}}
	c, err := Diff(context.Background(), old, new, tools.DnsxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Severity != SeverityReview {
		t.Fatalf("want 1 review finding, got %+v", c)
	}
}

func TestDiffInternalCNAMEIgnored(t *testing.T) {
	old := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Welcome", CNAME: []string{"bar.example.com"}}}
	new := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 404, Title: "Gone", CNAME: []string{"bar.example.com"}}}
	c, err := Diff(context.Background(), old, new, tools.DnsxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Fatalf("want 0 findings for internal cname, got %+v", c)
	}
}

func TestDiffStableHealthyNoFinding(t *testing.T) {
	old := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Welcome", CNAME: []string{"foo.example.com.herokudns.com"}}}
	new := []tools.HttpxResult{{Host: "foo.example.com", StatusCode: 200, Title: "Welcome", CNAME: []string{"foo.example.com.herokudns.com"}}}
	c, err := Diff(context.Background(), old, new, tools.DnsxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Fatalf("want 0 findings for stable healthy host, got %+v", c)
	}
}
