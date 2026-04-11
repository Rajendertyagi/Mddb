package main

import (
	"strings"
	"testing"
)

func TestNormalizePostcode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SW1A 1AA", "SW1A1AA"},
		{"sw1a1aa", "SW1A1AA"},
		{"  SW1A  1AA  ", "SW1A1AA"},
		{"75-001", "75001"},
		{"00-001", "00001"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizePostcode(c.in); got != c.want {
			t.Errorf("normalizePostcode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPostcodeLookupLoadFromReader(t *testing.T) {
	csv := strings.NewReader(`SW1A 1AA,51.501,-0.142
EC1A 1BB,51.518,-0.104
`)
	pl := NewPostcodeLookup()
	n, err := pl.loadFromReader("gb", csv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("loaded %d, want 2", n)
	}

	lat, lng, ok := pl.Resolve("GB", "sw1a 1aa")
	if !ok {
		t.Fatal("SW1A 1AA should resolve")
	}
	if lat != 51.501 || lng != -0.142 {
		t.Errorf("got (%v, %v), want (51.501, -0.142)", lat, lng)
	}

	// Case-insensitive country.
	_, _, ok = pl.Resolve("gb", "EC1A1BB")
	if !ok {
		t.Error("lowercase country code should work")
	}

	// Unknown postcode.
	_, _, ok = pl.Resolve("GB", "ZZZZ")
	if ok {
		t.Error("unknown postcode should not resolve")
	}

	// Unknown country.
	_, _, ok = pl.Resolve("US", "SW1A1AA")
	if ok {
		t.Error("unloaded country should not resolve")
	}
}

func TestPostcodeLookupResolveFromMeta(t *testing.T) {
	pl := NewPostcodeLookup()
	_, _ = pl.loadFromReader("PL", strings.NewReader(`00-001,52.231,21.006
`))

	cases := []struct {
		name   string
		meta   map[string][]string
		wantOK bool
	}{
		{"both present", map[string][]string{"geo_postcode": {"00-001"}, "geo_country": {"PL"}}, true},
		{"missing country", map[string][]string{"geo_postcode": {"00-001"}}, false},
		{"missing postcode", map[string][]string{"geo_country": {"PL"}}, false},
		{"empty", map[string][]string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, ok := pl.ResolveFromMeta(c.meta)
			if ok != c.wantOK {
				t.Errorf("got %v, want %v", ok, c.wantOK)
			}
		})
	}
}

func TestPostcodeLookupStatsAndCountries(t *testing.T) {
	pl := NewPostcodeLookup()
	_, _ = pl.loadFromReader("gb", strings.NewReader("A,0,0\nB,1,1\n"))
	_, _ = pl.loadFromReader("pl", strings.NewReader("C,2,2\n"))

	stats := pl.Stats()
	if stats["GB"] != 2 || stats["PL"] != 1 {
		t.Errorf("unexpected stats: %v", stats)
	}

	countries := pl.Countries()
	if len(countries) != 2 || countries[0] != "GB" || countries[1] != "PL" {
		t.Errorf("countries sorted snapshot wrong: %v", countries)
	}
}

func TestPostcodeLookupInvalidCSV(t *testing.T) {
	// Three required fields — short row fails.
	pl := NewPostcodeLookup()
	_, err := pl.loadFromReader("gb", strings.NewReader("SW1A1AA,51.5\n"))
	if err == nil {
		t.Error("expected error for short row")
	}

	// Bad lat parse.
	_, err = pl.loadFromReader("gb", strings.NewReader("SW1A1AA,not-a-number,0\n"))
	if err == nil {
		t.Error("expected error for bad lat")
	}
}

func TestPostcodeLookupEmptyCountry(t *testing.T) {
	pl := NewPostcodeLookup()
	_, err := pl.loadFromReader("", strings.NewReader("A,0,0\n"))
	if err == nil {
		t.Error("expected error for empty country")
	}
}
