package runner

import (
	"reflect"
	"testing"

	"github.com/pajarori/assetlas/internal/platform"
)

func TestExtractTargets(t *testing.T) {
	prog := platform.Program{
		Scopes: []platform.Scope{
			{Identifier: "*.foo.com", Type: platform.AssetWildcard, Eligible: true},
			{Identifier: "*.foo.com", Type: platform.AssetWildcard, Eligible: true},
			{Identifier: "https://api.foo.com/v1", Type: platform.AssetAPI, Eligible: true},
			{Identifier: "https://bar.com/path", Type: platform.AssetURL, Eligible: true},
			{Identifier: "*.skipme.com", Type: platform.AssetWildcard, Eligible: false},
			{Identifier: "com.example.app", Type: platform.AssetAndroid, Eligible: true},
			{Identifier: "id12345", Type: platform.AssetIOS, Eligible: true},
			{Identifier: "weird stuff with spaces", Type: platform.AssetWildcard, Eligible: true},
			{Identifier: " ", Type: platform.AssetURL, Eligible: true},
			{Identifier: "", Type: platform.AssetURL, Eligible: true},
		},
	}
	got := ExtractTargets(prog)
	want := []string{"api.foo.com", "bar.com", "foo.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTargets = %v; want %v", got, want)
	}
}

func TestExtractTargetsEmpty(t *testing.T) {
	if got := ExtractTargets(platform.Program{}); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestUniqueSortedLower(t *testing.T) {
	in := []string{"Foo.com", "foo.com", "  BAR.com  ", "bar.com", "", "  "}
	got := uniqueSortedLower(in)
	want := []string{"bar.com", "foo.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("uniqueSortedLower = %v; want %v", got, want)
	}
}

func TestTLSTargetsFromHosts(t *testing.T) {
	got := tlsTargetsFromHosts([]string{"a.com", "b.com"})
	want := []string{"a.com:443", "b.com:443"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tlsTargetsFromHosts = %v; want %v", got, want)
	}
}
