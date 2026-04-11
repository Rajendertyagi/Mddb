package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// PostcodeLookup is an opt-in in-memory postcode → (lat, lng) table,
// grouped by country. Users populate it via LoadCountry. It is attached
// to GeoIndex via SetPostcodes; if nil, AddFromMeta falls back to the
// standard no-op for docs that only carry geo_postcode/geo_country.
//
// Countries are identified by a free-form string (typically an ISO 3166-1
// alpha-2 code like "GB", "FR", "PL"). Postcode keys are normalized with
// normalizePostcode before lookup/insert so "SW1A 1AA" and "sw1a1aa" collide.
type PostcodeLookup struct {
	mu   sync.RWMutex
	data map[string]map[string][2]float64 // country → postcode → [lat, lng]
}

// NewPostcodeLookup creates an empty postcode lookup.
func NewPostcodeLookup() *PostcodeLookup {
	return &PostcodeLookup{
		data: make(map[string]map[string][2]float64),
	}
}

// LoadCountry parses a CSV file of `postcode,lat,lng` rows (no header) and
// registers it under the given country code. Existing entries for that
// country are replaced atomically.
func (pl *PostcodeLookup) LoadCountry(country, csvPath string) (int, error) {
	f, err := os.Open(csvPath) // #nosec G304 -- operator-supplied path
	if err != nil {
		return 0, fmt.Errorf("open postcode csv: %w", err)
	}
	defer func() { _ = f.Close() }()
	return pl.loadFromReader(country, f)
}

// loadFromReader is factored out for test injection.
func (pl *PostcodeLookup) loadFromReader(country string, r io.Reader) (int, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return 0, fmt.Errorf("empty country code")
	}
	table := make(map[string][2]float64)
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = 3
	reader.ReuseRecord = true
	row := 0
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		row++
		if err != nil {
			return 0, fmt.Errorf("csv row %d: %w", row, err)
		}
		postcode := normalizePostcode(rec[0])
		if postcode == "" {
			continue
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
		if err != nil {
			return 0, fmt.Errorf("csv row %d: lat: %w", row, err)
		}
		lng, err := strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		if err != nil {
			return 0, fmt.Errorf("csv row %d: lng: %w", row, err)
		}
		if !validLatLng(lat, lng) {
			continue
		}
		table[postcode] = [2]float64{lat, lng}
	}
	pl.mu.Lock()
	pl.data[country] = table
	pl.mu.Unlock()
	return len(table), nil
}

// Resolve looks up a (country, postcode) pair. Returns (0, 0, false) if the
// country is not loaded or the postcode is unknown.
func (pl *PostcodeLookup) Resolve(country, postcode string) (float64, float64, bool) {
	country = strings.ToUpper(strings.TrimSpace(country))
	postcode = normalizePostcode(postcode)
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	table, ok := pl.data[country]
	if !ok {
		return 0, 0, false
	}
	coord, ok := table[postcode]
	if !ok {
		return 0, 0, false
	}
	return coord[0], coord[1], true
}

// ResolveFromMeta pulls geo_postcode + geo_country out of a doc's meta and
// delegates to Resolve. Used by GeoIndex.AddFromMeta as the fallback path.
func (pl *PostcodeLookup) ResolveFromMeta(meta map[string][]string) (float64, float64, bool) {
	pcVals := meta[metaKeyGeoPostcode]
	ccVals := meta[metaKeyGeoCountry]
	if len(pcVals) == 0 || len(ccVals) == 0 {
		return 0, 0, false
	}
	return pl.Resolve(ccVals[0], pcVals[0])
}

// Stats returns per-country row counts for GET /v1/geo-stats.
func (pl *PostcodeLookup) Stats() map[string]int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	out := make(map[string]int, len(pl.data))
	for k, v := range pl.data {
		out[k] = len(v)
	}
	return out
}

// Countries returns a sorted snapshot of loaded country codes.
func (pl *PostcodeLookup) Countries() []string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	names := make([]string, 0, len(pl.data))
	for k := range pl.data {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// normalizePostcode upper-cases, trims, and strips internal whitespace.
// This makes "SW1A 1AA", "sw1a1aa", and " SW1A  1AA " all collide to the
// same key, which matches how most postal datasets are indexed.
func normalizePostcode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '-' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}
