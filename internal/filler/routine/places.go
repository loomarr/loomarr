// Package routine holds the unattended routines the filler jobs run: the ingest pipeline's driver
// and its detail pass, the retirement of clips no current place can air, the switching of curated
// sources for the places' nearby stations, and the open archive search those places gate. The
// composition root declares the jobs and hands each routine what it reads.
package routine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/source"
	archiveprovider "github.com/loomarr/loomarr/internal/filler/source/archive"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// currentAreas resolves every current place to its area: the installation's, then each channel's
// own (#2059 decisions 7 and 8). No areas means no installation country, so nothing is bounded.
func currentAreas(ctx context.Context, channels interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
}, policy pod.Policy) ([]pod.Area, error) {
	list, err := channels.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	places := make([]pod.Geography, 0, len(list))
	for _, ch := range list {
		if ch.Policy.Filler != nil && ch.Policy.Filler.Geography != nil {
			places = append(places, pod.Geography{Country: ch.Policy.Filler.Geography.Country,
				Place: ch.Policy.Filler.Geography.Place})
		}
	}
	return policy.UsableAreas(places), nil
}

// NearbyPlaceStations is every station within the radius of some current place.
func NearbyPlaceStations(ctx context.Context, channels interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
}, policy pod.Policy) (map[string]bool, error) {
	areas, err := currentAreas(ctx, channels, policy)
	if err != nil {
		return nil, err
	}
	near := map[string]bool{}
	for _, area := range areas {
		for id := range area.Stations {
			near[id] = true
		}
	}
	return near, nil
}

// LocalCuratedSources switches curated sources on for the stations near the current places, and
// back off when no place is near them any more (#2059 decision 10: setting a place is the consent
// for its local sources). A source the operator switched either way stays as they left it.
type LocalCuratedSources struct {
	store  CuratedSwitcher
	policy func() pod.Policy
	log    *slog.Logger
}

// CuratedSwitcher is the slice of the store the curated-source switch reads and writes.
type CuratedSwitcher interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	SwitchLocalCuratedSources(ctx context.Context, near map[string]bool) (on, off []string, err error)
}

// NewLocalCuratedSources returns the curated-source switch over store and the places' policy.
func NewLocalCuratedSources(store CuratedSwitcher, policy func() pod.Policy, log *slog.Logger) LocalCuratedSources {
	return LocalCuratedSources{store: store, policy: policy, log: log}
}

func (l LocalCuratedSources) Run(ctx context.Context) {
	if l.store == nil {
		return
	}
	near, err := NearbyPlaceStations(ctx, l.store, l.policy())
	if err != nil {
		l.log.Warn("filler: local curated sources not switched; places unreadable", "err", err)
		return
	}
	on, off, err := l.store.SwitchLocalCuratedSources(ctx, near)
	if err != nil {
		l.log.Warn("filler: local curated sources not switched", "err", err)
		return
	}
	if len(on) > 0 || len(off) > 0 {
		l.log.Info("filler: curated sources switched for the places' nearby stations", "on", on, "off", off)
	}
}

// OpenArchiveSearch searches all of archive.org for a nearby station (#2059 decisions 10 and 11).
// Its gate admits a hit whose title names a station near some current place, held on the title's
// date.
type OpenArchiveSearch struct {
	archive interface {
		Search(ctx context.Context, query string, limit int) (fetch.DiscoveryResult, error)
	}
	channels interface {
		ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	}
	policy   func() pod.Policy
	stations interface {
		Get(context.Context) (*station.Directory, error)
	}
	languages func(context.Context) (string, error)
}

// NewOpenArchiveSearch returns the open archive search. archive runs the query, channels name the
// current places, stations resolve a title to a station, and languages reads the wanted set.
func NewOpenArchiveSearch(
	archive interface {
		Search(ctx context.Context, query string, limit int) (fetch.DiscoveryResult, error)
	},
	channels interface {
		ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	},
	policy func() pod.Policy,
	stations interface {
		Get(context.Context) (*station.Directory, error)
	},
	languages func(context.Context) (string, error),
) OpenArchiveSearch {
	return OpenArchiveSearch{archive: archive, channels: channels, policy: policy, stations: stations, languages: languages}
}

func (o OpenArchiveSearch) OpenSearch(ctx context.Context, query string, limit int) ([]source.Item, error) {
	res, err := o.archive.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return archiveprovider.Items(res.Items), nil
}

// Provider is the provider an open-search hit is acquired from.
func (o OpenArchiveSearch) Provider() source.ID { return archiveprovider.ID }

func (o OpenArchiveSearch) Gate(ctx context.Context) (acquisition.ListingGate, error) {
	near, err := NearbyPlaceStations(ctx, o.channels, o.policy())
	if err != nil {
		return acquisition.ListingGate{}, err
	}
	directory, err := o.stations.Get(ctx)
	if err != nil {
		return acquisition.ListingGate{}, err
	}
	languages, err := o.languages(ctx)
	if err != nil {
		return acquisition.ListingGate{}, err
	}
	return acquisition.ListingGate{Languages: languages, Station: func(title string) (string, bool) {
		match, ok := directory.Identify(title, "", "", "")
		if !ok || !near[match.Station.ID] {
			return "", false
		}
		return match.CallSign, true
	}}, nil
}

// NearbyCallSigns are the call signs a short pass searches for (#2059 decision 9): every one a
// nearby station held during an era gap, or ever when there is none. Sorted, so the searches a
// pass rotates through are stable.
func NearbyCallSigns(near map[string]bool, eras []pod.EraRange, directory *station.Directory) []string {
	spans := [][2]string{{"", ""}}
	if len(eras) > 0 {
		spans = spans[:0]
		for _, r := range eras {
			var from, to string
			if r.From > 0 {
				from = fmt.Sprintf("%04d-01-01", r.From)
			}
			if r.To > 0 {
				to = fmt.Sprintf("%04d-01-01", r.To+1)
			}
			spans = append(spans, [2]string{from, to})
		}
	}
	seen := map[string]bool{}
	var out []string
	for id := range near {
		station, ok := directory.Station(id)
		if !ok {
			continue
		}
		for _, span := range spans {
			for _, sign := range station.CallSignsDuring(span[0], span[1]) {
				if !seen[sign] {
					seen[sign] = true
					out = append(out, sign)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}
