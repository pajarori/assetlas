package fetcher

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")
	d := parseRetryAfter(h)
	if d != 30*time.Second {
		t.Errorf("expected 30s, got %v", d)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	h := http.Header{}
	future := time.Now().Add(45 * time.Second).UTC().Format(http.TimeFormat)
	h.Set("Retry-After", future)
	d := parseRetryAfter(h)
	if d <= 0 || d > 60*time.Second {
		t.Errorf("expected ~45s, got %v", d)
	}
}

func TestParseRetryAfterEmpty(t *testing.T) {
	if d := parseRetryAfter(http.Header{}); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "garbage")
	if d := parseRetryAfter(h); d != 0 {
		t.Errorf("expected 0 for garbage, got %v", d)
	}
}

func TestShouldRetry(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{200, false},
		{301, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{599, true},
		{600, false},
	}
	for _, c := range cases {
		if got := shouldRetry(c.code); got != c.want {
			t.Errorf("shouldRetry(%d) = %v; want %v", c.code, got, c.want)
		}
	}
}

func TestHostOf(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com:8080", "example.com:8080"},
		{"https://api.foo.com", "api.foo.com"},
		{"not a url", "not a url"},
	}
	for _, c := range cases {
		if got := hostOf(c.url); got != c.want {
			t.Errorf("hostOf(%q) = %q; want %q", c.url, got, c.want)
		}
	}
}
