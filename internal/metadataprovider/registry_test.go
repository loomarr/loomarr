package metadataprovider_test

import (
	"context"
	"reflect"
	"testing"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

type fake struct {
	id   mp.ID
	caps mp.Capabilities
}

func (f fake) ID() mp.ID                     { return f.id }
func (f fake) Capabilities() mp.Capabilities { return f.caps }
func (fake) Fetch(context.Context, mp.Ref) (mp.Record, error) {
	return mp.Record{}, mp.ErrNotFound
}

func caps(basis mp.Basis, families []mp.Family, resolves ...mp.Resolvable) mp.Capabilities {
	var kinds []mp.Kind
	for _, r := range resolves {
		kinds = append(kinds, r.Kind)
	}
	return mp.Capabilities{Basis: basis, Kinds: kinds, Resolves: resolves, Families: families}
}

var (
	seriesByTVDB = mp.Resolvable{Kind: mp.KindSeries, Namespace: mp.NamespaceTVDB}
	movieByTMDB  = mp.Resolvable{Kind: mp.KindMovie, Namespace: mp.NamespaceTMDB}
	movieByFile  = mp.Resolvable{Kind: mp.KindMovie, Namespace: mp.NamespaceFile}
)

func TestRegistry_RoutesByCapability(t *testing.T) {
	sonarr := fake{id: mp.IDSonarr, caps: caps(mp.BasisManager, []mp.Family{mp.FamilyIdentity, mp.FamilyTechnical}, seriesByTVDB)}
	radarr := fake{id: mp.IDRadarr, caps: caps(mp.BasisManager, []mp.Family{mp.FamilyIdentity, mp.FamilyCertification}, movieByTMDB)}
	nfo := fake{id: mp.IDNFO, caps: caps(mp.BasisSidecar, []mp.Family{mp.FamilyCertification}, movieByFile)}
	r, err := mp.NewRegistry(sonarr, radarr, nfo)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Resolving(mp.Ref{Kind: mp.KindMovie, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: "90000501"}}); !reflect.DeepEqual(got, []mp.Provider{radarr}) {
		t.Errorf("Resolving(movie tmdb) = %v", got)
	}
	if got := r.Resolving(mp.Ref{Kind: mp.KindEpisode, ID: mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: "90001001"}}); got != nil {
		t.Errorf("Resolving(episode tvdb) = %v, want none: no provider fetches an episode by its own id", got)
	}
	if p, ok := r.Get(mp.IDNFO); !ok || !reflect.DeepEqual(p, mp.Provider(nfo)) {
		t.Errorf("Get(nfo) = %v, %v", p, ok)
	}
	if got := r.Providers(); !reflect.DeepEqual(got, []mp.Provider{sonarr, radarr, nfo}) {
		t.Errorf("Providers() = %v, want registration order", got)
	}
}

func TestRegistry_AnUnregisteredProviderIsAbsent(t *testing.T) {
	r, err := mp.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get(mp.IDSonarr); ok {
		t.Error("an unregistered provider was found")
	}
	var nilRegistry *mp.Registry
	if _, ok := nilRegistry.Get(mp.IDSonarr); ok || nilRegistry.Providers() != nil {
		t.Error("a nil registry holds providers")
	}
}

func TestNewRegistry_RefusesAnUnknownRepeatedOrIncompleteProvider(t *testing.T) {
	good := caps(mp.BasisManager, []mp.Family{mp.FamilyIdentity}, movieByTMDB)
	cases := map[string][]mp.Provider{
		"empty id":      {fake{id: "", caps: good}},
		"unknown id":    {fake{id: "plex", caps: good}},
		"repeated id":   {fake{id: mp.IDRadarr, caps: good}, fake{id: mp.IDRadarr, caps: good}},
		"no basis":      {fake{id: mp.IDRadarr, caps: caps("", []mp.Family{mp.FamilyIdentity}, movieByTMDB)}},
		"no families":   {fake{id: mp.IDRadarr, caps: caps(mp.BasisManager, nil, movieByTMDB)}},
		"resolves none": {fake{id: mp.IDRadarr, caps: mp.Capabilities{Basis: mp.BasisManager, Kinds: []mp.Kind{mp.KindMovie}, Families: []mp.Family{mp.FamilyIdentity}}}},
	}
	for name, providers := range cases {
		if _, err := mp.NewRegistry(providers...); err == nil {
			t.Errorf("%s: registered", name)
		}
	}
}

func TestIDs_IsTheClosedSet(t *testing.T) {
	for _, id := range mp.IDs() {
		got, err := mp.ParseID(string(id))
		if err != nil || got != id {
			t.Errorf("ParseID(%q) = %q, %v", id, got, err)
		}
	}
	if _, err := mp.ParseID("tvdb"); err == nil {
		t.Error("ParseID accepted a provider that is not in the set")
	}
}

func TestRecord_FamiliesReportsWhatItSays(t *testing.T) {
	rec := mp.Record{
		IDs:           []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000501"}},
		Certification: &mp.Certification{Rating: "PG"},
	}
	if got, want := rec.Families(), []mp.Family{mp.FamilyIdentity, mp.FamilyCertification}; !reflect.DeepEqual(got, want) {
		t.Errorf("Families() = %v, want %v", got, want)
	}
	if got := (mp.Record{}).Families(); got != nil {
		t.Errorf("an empty record says %v", got)
	}
}
