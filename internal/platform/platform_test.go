package platform

import (
	"testing"
)

func TestDetectAssetType(t *testing.T) {
	cases := []struct {
		category   string
		identifier string
		want       AssetType
	}{
		{"url", "example.com", AssetURL},
		{"website", "example.com", AssetURL},
		{"web", "example.com", AssetURL},
		{"web-application", "example.com", AssetURL},
		{"url", "*.example.com", AssetWildcard},
		{"wildcard", "*.example.com", AssetWildcard},
		{"api", "api.example.com", AssetAPI},
		{"android", "com.example.app", AssetAndroid},
		{"google_play", "com.example.app", AssetAndroid},
		{"google_play_app_id", "com.example.app", AssetAndroid},
		{"ios", "1234567890", AssetIOS},
		{"apple_store", "id1234567890", AssetIOS},
		{"apple_store_app_id", "id1234567890", AssetIOS},
		{"source_code", "github.com/foo/bar", AssetSource},
		{"hardware", "Model X", AssetHardware},
		{"device", "Router Y", AssetHardware},
		{"", "*.example.com", AssetWildcard},
		{"", "com.example.app", AssetAndroid},
		{"", "https://play.google.com/store/apps/details?id=com.foo", AssetAndroid},
		{"", "https://apps.apple.com/app/id123", AssetIOS},
		{"", "https://itunes.apple.com/app/id123", AssetIOS},
		{"", "example.app.apk", AssetAndroid},
		{"", "example.ipa", AssetIOS},
		{"", "https://example.com", AssetURL},
		{"", "example.com", AssetOther},
		{"URL", "https://apps.apple.com/us/app/foo/id123", AssetIOS},
		{"URL", "https://play.google.com/store/apps/details?id=com.foo", AssetAndroid},
		{"url", "https://itunes.apple.com/app/foo", AssetIOS},
		{"website", "https://apps.apple.com/in/app/id742", AssetIOS},
		{"web", "https://play.google.com/store/apps/details?id=com.bar", AssetAndroid},
		{"URL", "https://cms-foo.ipaasferrero.com/admin", AssetURL},
	}
	for _, c := range cases {
		got := DetectAssetType(c.category, c.identifier)
		if got != c.want {
			t.Errorf("DetectAssetType(%q, %q) = %s; want %s", c.category, c.identifier, got, c.want)
		}
	}
}

func TestNormalizeWildcard(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  https://EXAMPLE.com/  ", "example.com"},
		{"http://Foo.com", "foo.com"},
		{"*.example.com", "*.example.com"},
		{"EXAMPLE.COM/path", "example.com/path"},
		{"  ", ""},
	}
	for _, c := range cases {
		got := NormalizeWildcard(c.in)
		if got != c.want {
			t.Errorf("NormalizeWildcard(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestStripURLPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"example.com", "example.com"},
		{"example.com/foo", "example.com"},
		{"example.com:443/foo/bar", "example.com:443"},
		{"/just/path", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := StripURLPath(c.in)
		if got != c.want {
			t.Errorf("StripURLPath(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "  ", "foo", "bar"}, "foo"},
		{[]string{"  foo  ", "bar"}, "foo"},
		{[]string{"", "  ", ""}, ""},
		{[]string{}, ""},
		{nil, ""},
	}
	for i, c := range cases {
		got := FirstNonEmpty(c.in...)
		if got != c.want {
			t.Errorf("case %d FirstNonEmpty(%v) = %q; want %q", i, c.in, got, c.want)
		}
	}
}

func TestIsKnownPlatform(t *testing.T) {
	for _, p := range AllPlatforms {
		if !IsKnownPlatform(p.Key) {
			t.Errorf("AllPlatforms entry %q not recognized by IsKnownPlatform", p.Key)
		}
	}
	if IsKnownPlatform("not-a-platform") {
		t.Error("IsKnownPlatform should return false for unknown name")
	}
}

func TestSortPrograms(t *testing.T) {
	progs := []Program{
		{Platform: "bugcrowd", Handle: "Zeta", Scopes: nil},
		{Platform: "bugcrowd", Handle: "alpha", Scopes: []Scope{{Identifier: "b.com"}, {Identifier: "a.com"}}},
		{Platform: "hackerone", Handle: "X"},
	}
	SortPrograms(progs)
	if progs[0].Platform != "bugcrowd" || progs[0].Handle != "alpha" {
		t.Errorf("expected bugcrowd/alpha first, got %+v", progs[0])
	}
	if progs[1].Platform != "bugcrowd" || progs[1].Handle != "Zeta" {
		t.Errorf("expected bugcrowd/Zeta second, got %+v", progs[1])
	}
	if progs[0].Scopes[0].Identifier != "a.com" {
		t.Errorf("scopes not sorted: %+v", progs[0].Scopes)
	}
	if progs[2].Scopes == nil {
		t.Errorf("expected nil scopes normalized to []")
	}
}
