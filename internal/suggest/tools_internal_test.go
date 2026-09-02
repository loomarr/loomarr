package suggest

import (
	"strings"
	"testing"
)

func TestParseDiscoveryQueryValidatesAndNormalizesScalarQualifiers(t *testing.T) {
	got, discovery, err := parseDiscoveryQuery(map[string]any{
		"genres":            []any{"Drama"},
		"original_language": " EN ",
		"origin_country":    "gb",
		"runtime_min":       float64(20),
		"runtime_max":       90,
		"vote_average_min":  7.5,
		"vote_count_min":    int64(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery || got.OriginalLanguage != "en" || got.OriginCountry != "GB" ||
		got.RuntimeMin != 20 || got.RuntimeMax != 90 || got.VoteAverageMin != 7.5 || got.VoteCountMin != 100 {
		t.Fatalf("normalized discovery query = %+v discovery=%v", got, discovery)
	}
}

func TestParseDiscoveryQueryRejectsMalformedOrBroadeningInputs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "bad language", args: map[string]any{"genres": []any{"Drama"}, "original_language": "english"}, want: "two-letter"},
		{name: "bad era", args: map[string]any{"era": "whenever"}, want: "year, decade"},
		{name: "non-string country", args: map[string]any{"genres": []any{"Drama"}, "origin_country": 44}, want: "two-letter"},
		{name: "fractional runtime", args: map[string]any{"genres": []any{"Drama"}, "runtime_min": 20.5}, want: "integer"},
		{name: "inverted runtime", args: map[string]any{"genres": []any{"Drama"}, "runtime_min": 90, "runtime_max": 20}, want: "must not exceed"},
		{name: "vote average above scale", args: map[string]any{"genres": []any{"Drama"}, "vote_average_min": 10.1}, want: "at most 10"},
		{name: "zero vote average", args: map[string]any{"genres": []any{"Drama"}, "vote_average_min": 0}, want: "greater than 0"},
		{name: "mixed search modes", args: map[string]any{"query": "Alien", "origin_country": "US"}, want: "cannot be combined"},
		{name: "empty request", args: map[string]any{}, want: "provide query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseDiscoveryQuery(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parse error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
