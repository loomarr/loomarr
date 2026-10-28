package app

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

type retirementFake struct {
	settled  []fillerstore.SettledGeographyClip
	channels []store.Channel
	removed  []string
}

func (f *retirementFake) ListSettledGeographyClips(context.Context) ([]fillerstore.SettledGeographyClip, error) {
	return f.settled, nil
}

func (f *retirementFake) ListChannels(context.Context) ([]store.Channel, error) {
	return f.channels, nil
}

func (f *retirementFake) SetClipsRemoved(_ context.Context, paths []string, _ time.Time) (int, error) {
	f.removed = append(f.removed, paths...)
	return len(paths), nil
}

func channelAt(country, place string) store.Channel {
	var ch store.Channel
	ch.Policy.Filler = &schedule.FillerSelection{Geography: &schedule.FillerGeography{Country: country, Place: place}}
	return ch
}

// Invented places: "harbor" reaches fcc:1, "lakeside" reaches fcc:3.
func retirementPolicy(country string) func() filler.Policy {
	return func() filler.Policy {
		return filler.Policy{Geography: filler.Geography{Country: country, Place: "harbor"},
			LocalStations: func(_, place string) map[string]bool {
				return map[string]map[string]bool{"harbor": {"fcc:1": true}, "lakeside": {"fcc:3": true}}[place]
			}}
	}
}

func TestGeographyRetirementRemovesOnlyWhatNoPlaceCanAir(t *testing.T) {
	fake := &retirementFake{
		settled: []fillerstore.SettledGeographyClip{
			{Path: "harbor-news.mp4", Scope: "local", Country: "US", Station: "fcc:1"},
			{Path: "lakeside-news.mp4", Scope: "local", Country: "US", Station: "fcc:3"},
			{Path: "faraway-news.mp4", Scope: "local", Country: "US", Station: "fcc:9"},
			{Path: "us-ad.mp4", Scope: "national", Country: "US"},
			{Path: "foreign-ad.mp4", Scope: "national", Country: "GB"},
		},
		channels: []store.Channel{channelAt("US", "lakeside"), channelAt("", "")},
	}
	retire := fillerGeographyRetirement{clips: fake, store: fake, policy: retirementPolicy("US"),
		log: slog.New(slog.NewTextHandler(io.Discard, nil)), now: time.Now}
	removed, err := retire.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(fake.removed)
	if removed != 2 || !slices.Equal(fake.removed, []string{"faraway-news.mp4", "foreign-ad.mp4"}) {
		t.Fatalf("removed %v; want only the clips no place can air", fake.removed)
	}
}

func TestGeographyRetirementNeedsAnInstallationCountry(t *testing.T) {
	fake := &retirementFake{settled: []fillerstore.SettledGeographyClip{
		{Path: "foreign-ad.mp4", Scope: "national", Country: "GB"},
	}}
	retire := fillerGeographyRetirement{clips: fake, store: fake, policy: retirementPolicy(""),
		log: slog.New(slog.NewTextHandler(io.Discard, nil)), now: time.Now}
	if removed, err := retire.Run(t.Context()); err != nil || removed != 0 || len(fake.removed) != 0 {
		t.Fatalf("removed %d (%v), %v; want nothing without an installation country", removed, fake.removed, err)
	}
}
