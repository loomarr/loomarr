package identity

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/media/tools"
)

type identityMemStore struct {
	clips        []IdentityClip
	fingerprints map[string]creative.Fingerprint
	variants     map[string][]string
	merges       []InferiorCopy
	links        [][2]string
	wanted       []WantedCopy
}

func (m *identityMemStore) ListIdentityClips(context.Context) ([]IdentityClip, error) {
	return m.clips, nil
}
func (m *identityMemStore) ListClipFingerprints(context.Context, string) (map[string]creative.Fingerprint, error) {
	return m.fingerprints, nil
}
func (m *identityMemStore) UpsertClipFingerprint(_ context.Context, hash, _ string, fp creative.Fingerprint) error {
	m.fingerprints[hash] = fp
	return nil
}
func (m *identityMemStore) ListClipVariants(context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	for k, v := range m.variants {
		out[k] = append([]string(nil), v...)
	}
	return out, nil
}
func (m *identityMemStore) LinkClipVariants(_ context.Context, a, b string) error {
	m.links = append(m.links, [2]string{a, b})
	m.variants[a], m.variants[b] = append(m.variants[a], b), append(m.variants[b], a)
	return nil
}
func (m *identityMemStore) MergeInferiorCopy(_ context.Context, c InferiorCopy) error {
	m.merges = append(m.merges, c)
	m.wanted = slices.DeleteFunc(m.wanted, func(w WantedCopy) bool { return w.ClipHash == c.ClipHash })
	return nil
}
func (m *identityMemStore) ListWantedCopies(context.Context) ([]WantedCopy, error) {
	return slices.Clone(m.wanted), nil
}

// identityTools fingerprints each clip from a table keyed by its content hash.
type identityTools struct {
	spanTools
	byHash map[string]creative.Fingerprint
	calls  int
}

func (f *identityTools) Fingerprint(_ context.Context, path string, _, _ int64) (creative.Fingerprint, error) {
	f.calls++
	return f.byHash[strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))], nil
}

// identitySpot is a 30 s advert's fingerprint, one picture per 2.5 s shot; the same seed is the
// same advert.
func identitySpot(seed uint64) creative.Fingerprint {
	r := rand.New(rand.NewPCG(seed, 2033))
	var video []uint64
	for range 12 {
		shot := r.Uint64() | 1<<63 | 0xff
		for range 10 {
			video = append(video, shot)
		}
	}
	return creative.Fingerprint{DurationMs: 30_000, Video: video}
}

// Same tolerates a few seconds of difference, so it does not chain: a dealer tag on B's end and
// another on C's start leave B matching both A and C while A and C differ by twice that. Only the
// copy that matches the survivor directly folds into it; C stays its own clip, linked as a version.
func TestCreativeIdentity_MergesOnlyCopiesThatMatchTheSurvivorDirectly(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	r := rand.New(rand.NewPCG(9, 2033))
	retag := func(fp creative.Fingerprint, from, to int) creative.Fingerprint {
		out := creative.Fingerprint{DurationMs: fp.DurationMs, Video: slices.Clone(fp.Video)}
		for i := from; i < to; i++ {
			out.Video[i] = r.Uint64() | 1<<63 | 0xff
		}
		return out
	}
	a := identitySpot(3)
	b := retag(a, 105, 120)
	c := retag(b, 0, 15)
	if creative.Compare(a, b) != creative.Same || creative.Compare(b, c) != creative.Same || creative.Compare(a, c) == creative.Same {
		t.Fatalf("fixture: want A~B and B~C but not A~C; got %v %v %v",
			creative.Compare(a, b), creative.Compare(b, c), creative.Compare(a, c))
	}

	hashes := map[string]creative.Fingerprint{}
	var pool []IdentityClip
	for i, spec := range []struct {
		fp      creative.Fingerprint
		quality string
	}{{a, "480p"}, {b, "240p"}, {c, "240p"}} {
		hash := writeContentAddressedClip(t, dir, []byte{byte(i), 'x'}, ".mp4")
		hashes[hash] = spec.fp
		ic := IdentityClip{CreatedAt: t0.Add(time.Duration(i) * time.Hour)}
		ic.Hash, ic.Path, ic.Quality, ic.DurationMs, ic.Role = hash, cliplibrary.ClipRelPath(hash, ".mp4"), spec.quality, 30_000, clips.Commercial
		ic.Placement = clips.PlacementBreakBody
		pool = append(pool, ic)
	}
	st := &identityMemStore{clips: pool, fingerprints: map[string]creative.Fingerprint{}, variants: map[string][]string{}}
	res, err := NewJob(st, &identityTools{byHash: hashes}, dir, nil, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged != 1 || len(st.merges) != 1 || st.merges[0].ClipHash != pool[1].Hash || st.merges[0].SurvivorHash != pool[0].Hash {
		t.Fatalf("merges = %+v, want only B folded into A", st.merges)
	}
	if !reflect.DeepEqual(st.variants[pool[2].Hash], []string{pool[0].Hash}) {
		t.Fatalf("variants = %v, want C linked to A", st.variants)
	}
}

// One pass over a catalog holding an ad twice and a cut-down of it: the sharper copy survives even
// though the other arrived first, the other is merged with its source identity and its own files
// (media, sidecar, source master) deleted, and the cut-down is linked to the survivor.
// #2036: a copy of an ad the floor retired arrives cleaner from another source and takes the
// retired copy's place, inheriting its rotation through the merge. Between two published copies,
// the one with fewer defect findings wins even over a higher resolution.
func TestCreativeIdentity_UpgradesAWantedAdAndPrefersTheCleanerCopy(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	clip := func(body, quality string, a *defects.Assessment) IdentityClip {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		c := IdentityClip{CreatedAt: t0, Defects: a}
		c.Hash, c.Path, c.Quality, c.DurationMs, c.Role = hash, cliplibrary.ClipRelPath(hash, ".mp4"), quality, 30_000, clips.Commercial
		c.Placement = clips.PlacementBreakBody
		return c
	}
	clean := &defects.Assessment{Outcome: defects.Pass, Findings: []defects.Finding{{Check: defects.CheckBlur, Outcome: defects.Pass}}}
	soft := &defects.Assessment{Outcome: defects.Borderline, Findings: []defects.Finding{{Check: defects.CheckBlur, Outcome: defects.Borderline}}}

	// A retired copy is a row whose file the floor already deleted.
	retired := func(body string) WantedCopy {
		c := clip(body, "", nil)
		if err := os.Remove(filepath.Join(dir, c.Path)); err != nil {
			t.Fatal(err)
		}
		return WantedCopy{ClipHash: c.Hash, Path: c.Path}
	}
	arrival := clip("the soap ad, clean, from a second source", "480p", clean)
	cleanLow := clip("the cola ad, clean", "360p", clean)
	softHigh := clip("the cola ad, sharper but smeared", "480p", soft)
	retiredSoap, unmatched := retired("the soap ad, blocky"), retired("an ad nothing has matched")
	// Its only new copy fails the floor as well: no upgrade, and the floor retires that one next.
	retiredJuice := retired("the juice ad, blocky")
	blockyJuice := clip("the juice ad, blocky again", "480p", &defects.Assessment{Outcome: defects.Fail})
	soap, cola, juice := identitySpot(1), identitySpot(2), identitySpot(4)
	st := &identityMemStore{
		clips: []IdentityClip{arrival, cleanLow, softHigh, blockyJuice},
		fingerprints: map[string]creative.Fingerprint{retiredSoap.ClipHash: soap, unmatched.ClipHash: identitySpot(3),
			retiredJuice.ClipHash: juice},
		variants: map[string][]string{},
		wanted:   []WantedCopy{retiredSoap, unmatched, retiredJuice},
	}
	tools := &identityTools{byHash: map[string]creative.Fingerprint{arrival.Hash: soap, cleanLow.Hash: cola, softHigh.Hash: cola,
		blockyJuice.Hash: juice}}

	res, err := NewJob(st, tools, dir, nil, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upgraded != 1 || res.Merged != 1 {
		t.Fatalf("pass = %+v, want 1 upgrade and 1 merge", res)
	}
	want := []InferiorCopy{
		{ClipHash: softHigh.Hash, SourceHash: softHigh.Hash, SurvivorHash: cleanLow.Hash, DecidedAt: t0},
		{ClipHash: retiredSoap.ClipHash, SourceHash: retiredSoap.ClipHash, SurvivorHash: arrival.Hash, DecidedAt: t0},
	}
	if !reflect.DeepEqual(st.merges, want) {
		t.Fatalf("merges = %+v, want the smeared copy into the clean one, and the retired soap into its arrival", st.merges)
	}
	if len(st.wanted) != 2 || st.wanted[0].ClipHash != unmatched.ClipHash || st.wanted[1].ClipHash != retiredJuice.ClipHash {
		t.Fatalf("still wanted = %+v, want the ad nothing has matched and the one matched only by a failing copy", st.wanted)
	}
}

// sharedMasterFixture is a catalog of a sharp copy, a soft copy of the same ad that names a source
// master, and a third, unrelated clip. The third clip's sidecar is whatever neighbour writes: the
// unreadable sidecar is the one that might name the soft copy's master too.
type sharedMasterFixture struct {
	dir                           string
	store                         *identityMemStore
	fakeTools                     *identityTools
	soft, sharp, neighbour        string
	softFull, neighbourFull       string
	masterFull                    string
	master                        asset.Identity
	neighbourSidecar, softSidecar string
}

func newSharedMasterFixture(t *testing.T, neighbour func(t *testing.T, f *sharedMasterFixture)) *sharedMasterFixture {
	t.Helper()
	f := &sharedMasterFixture{dir: t.TempDir()}
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	f.sharp = writeContentAddressedClip(t, f.dir, []byte("the 480p copy"), ".mp4")
	f.soft = writeContentAddressedClip(t, f.dir, []byte("the 240p copy"), ".mp4")
	f.neighbour = writeContentAddressedClip(t, f.dir, []byte("an unrelated ad"), ".mp4")
	f.softFull = filepath.Join(f.dir, cliplibrary.ClipRelPath(f.soft, ".mp4"))
	f.neighbourFull = filepath.Join(f.dir, cliplibrary.ClipRelPath(f.neighbour, ".mp4"))
	f.softSidecar, f.neighbourSidecar = tools.SidecarPathFor(f.softFull), tools.SidecarPathFor(f.neighbourFull)

	sourceFile := filepath.Join(f.dir, "soft-source.mkv")
	if err := os.WriteFile(sourceFile, []byte("the soft copy's downloaded bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceHash, err := asset.ClipID(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if f.master, err = preserveSourceMaster(ctx, f.dir, sourceFile, sourceHash, asset.SidecarTags{}); err != nil {
		t.Fatal(err)
	}
	f.masterFull = filepath.Join(f.dir, filepath.FromSlash(f.master.Path))
	if err := asset.WriteSidecarTags(f.softFull, asset.SidecarTags{MediaAssets: &asset.Manifest{
		Version: asset.ManifestVersion, SourceMaster: f.master,
	}}, false); err != nil {
		t.Fatal(err)
	}
	neighbour(t, f)

	clip := func(hash, quality string, created time.Time) IdentityClip {
		c := IdentityClip{CreatedAt: created}
		c.Hash, c.Path, c.Quality, c.DurationMs, c.Role = hash, cliplibrary.ClipRelPath(hash, ".mp4"), quality, 30_000, clips.Commercial
		c.Placement = clips.PlacementBreakBody
		return c
	}
	f.store = &identityMemStore{
		clips: []IdentityClip{clip(f.soft, "240p", t0), clip(f.sharp, "480p", t0.Add(time.Hour)),
			clip(f.neighbour, "480p", t0.Add(2*time.Hour))},
		fingerprints: map[string]creative.Fingerprint{},
		variants:     map[string][]string{},
	}
	f.fakeTools = &identityTools{byHash: map[string]creative.Fingerprint{
		f.sharp: identitySpot(1), f.soft: identitySpot(1), f.neighbour: identitySpot(7)}}
	return f
}

func (f *sharedMasterFixture) run(t *testing.T) (Result, string) {
	t.Helper()
	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, nil))
	t0 := time.Unix(1_800_000_000, 0).UTC()
	res, err := NewJob(f.store, f.fakeTools, f.dir, nil, func() time.Time { return t0 }, log).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res, logged.String()
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// The master of an inferior copy goes with it only when no other clip names it. A sidecar that
// exists but cannot be read might name it, so the pass keeps the master, deletes what is certainly
// the loser's own, and says which sidecar it could not read.
func TestCreativeIdentity_AnUnreadableSidecarKeepsTheMasterTheLoserNames(t *testing.T) {
	cases := map[string]func(t *testing.T, f *sharedMasterFixture){
		"corrupt json": func(t *testing.T, f *sharedMasterFixture) {
			if err := os.WriteFile(f.neighbourSidecar, []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"invalid loomarr object": func(t *testing.T, f *sharedMasterFixture) {
			if err := os.WriteFile(f.neighbourSidecar, []byte(`{"loomarr":null}`), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"sidecar is a directory": func(t *testing.T, f *sharedMasterFixture) {
			if err := os.Mkdir(f.neighbourSidecar, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"clip's shard directory is a file": func(t *testing.T, f *sharedMasterFixture) {
			// The sidecar path cannot be listed at all: ENOTDIR, not ENOENT.
			if err := os.Remove(f.neighbourFull); err != nil {
				t.Fatal(err)
			}
			shard := filepath.Dir(f.neighbourFull)
			if err := os.Remove(shard); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(shard, []byte("not a directory"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, neighbour := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSharedMasterFixture(t, neighbour)
			res, logged := f.run(t)
			if res.Merged != 1 {
				t.Fatalf("pass = %+v, want the soft copy merged", res)
			}
			if !fileExists(f.masterFull) {
				t.Fatalf("the master %s was deleted although an unreadable sidecar might name it", f.masterFull)
			}
			for _, gone := range []string{f.softFull, f.softSidecar} {
				if fileExists(gone) {
					t.Errorf("the merged copy's own file %s survived", gone)
				}
			}
			if !strings.Contains(logged, "level=WARN") || !strings.Contains(logged, filepath.Base(f.neighbourSidecar)) {
				t.Errorf("log = %q, want a warning naming the unreadable sidecar", logged)
			}
		})
	}
}

// A clip with no sidecar names nothing, so it does not hold a master; only the last reference
// deletes it.
func TestCreativeIdentity_AMissingSidecarDoesNotHoldTheMaster(t *testing.T) {
	f := newSharedMasterFixture(t, func(*testing.T, *sharedMasterFixture) {})
	res, logged := f.run(t)
	if res.Merged != 1 {
		t.Fatalf("pass = %+v, want the soft copy merged", res)
	}
	if fileExists(f.masterFull) {
		t.Fatal("the last reference to the master was merged away and the master stayed")
	}
	if strings.Contains(logged, "level=WARN") {
		t.Errorf("log = %q, want no warning for a clip that has no sidecar", logged)
	}
}

// A readable sidecar that names the same master still shares it.
func TestCreativeIdentity_AReadableSharerStillHoldsTheMaster(t *testing.T) {
	f := newSharedMasterFixture(t, func(t *testing.T, f *sharedMasterFixture) {
		if err := asset.WriteSidecarTags(f.neighbourFull, asset.SidecarTags{MediaAssets: &asset.Manifest{
			Version: asset.ManifestVersion, SourceMaster: f.master,
		}}, false); err != nil {
			t.Fatal(err)
		}
	})
	if res, _ := f.run(t); res.Merged != 1 {
		t.Fatalf("pass = %+v, want the soft copy merged", res)
	}
	if !fileExists(f.masterFull) {
		t.Fatal("the master was deleted although a readable sidecar names it")
	}
}

func TestReferencedAssets_SeparatesAbsentFromUnreadable(t *testing.T) {
	f := newSharedMasterFixture(t, func(*testing.T, *sharedMasterFixture) {})
	paths := []string{cliplibrary.ClipRelPath(f.soft, ".mp4"), cliplibrary.ClipRelPath(f.neighbour, ".mp4")}

	refs := ReferencedAssets(f.dir, paths, nil)
	if len(refs.Unreadable()) != 0 || !refs.Claim(f.master.Path) {
		t.Fatalf("with one sidecar absent: unreadable %v, want none, and the lone reference claimable", refs.Unreadable())
	}

	if err := os.WriteFile(f.neighbourSidecar, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs = ReferencedAssets(f.dir, paths, nil)
	if got := refs.Unreadable(); len(got) != 1 || got[0] != f.neighbourSidecar {
		t.Fatalf("unreadable = %v, want only %s", got, f.neighbourSidecar)
	}
	if refs.Claim(f.master.Path) {
		t.Fatal("the master was claimed as unshared while a sidecar is unreadable")
	}
	if _, err := OwnFiles(f.dir, paths[0], asset.SidecarTags{MediaAssets: &asset.Manifest{SourceMaster: f.master}}, refs); !errors.Is(err, ErrReferencesUnknown) {
		t.Fatalf("OwnFiles error = %v, want ErrReferencesUnknown", err)
	}
	failed := RemoveRetiredCopy(f.dir, paths[0], asset.SidecarTags{MediaAssets: &asset.Manifest{SourceMaster: f.master}}, refs)
	if err := failed[f.masterFull]; !errors.Is(err, ErrReferencesUnknown) || !strings.Contains(err.Error(), f.neighbourSidecar) {
		t.Fatalf("RemoveRetiredCopy failures = %v, want the master reported kept, naming the sidecar", failed)
	}
	if !fileExists(f.masterFull) {
		t.Fatal("RemoveRetiredCopy deleted the master")
	}

	// A path that resolves to no sidecar location cannot be shown to name nothing.
	if got := ReferencedAssets(f.dir, []string{"../outside.mp4"}, nil).Unreadable(); len(got) != 1 {
		t.Fatalf("unreadable = %v, want the unresolvable path listed", got)
	}
	var none *AssetRefs
	if none.Claim(f.master.Path) || none.Unreadable() != nil {
		t.Fatal("the nil count claimed an asset")
	}
}

func TestCreativeIdentity_KeepsTheBestCopyAndLinksTheCutDown(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()

	sharp := writeContentAddressedClip(t, dir, []byte("the 480p copy"), ".mp4")
	soft := writeContentAddressedClip(t, dir, []byte("the 240p copy, admitted first"), ".mp4")
	cutDown := writeContentAddressedClip(t, dir, []byte("the 15 s cut-down"), ".mp4")
	softFull := filepath.Join(dir, cliplibrary.ClipRelPath(soft, ".mp4"))

	sourceFile := filepath.Join(dir, "soft-source.mkv")
	if err := os.WriteFile(sourceFile, []byte("the soft copy's downloaded bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceHash, err := asset.ClipID(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	master, err := preserveSourceMaster(ctx, dir, sourceFile, sourceHash, asset.SidecarTags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := asset.WriteSidecarTags(softFull, asset.SidecarTags{MediaAssets: &asset.Manifest{
		Version: asset.ManifestVersion, SourceMaster: master,
	}}, false); err != nil {
		t.Fatal(err)
	}
	masterFull := filepath.Join(dir, filepath.FromSlash(master.Path))

	clip := func(hash, quality string, durationMs int64, created time.Time) IdentityClip {
		c := IdentityClip{CreatedAt: created}
		c.Hash, c.Path, c.Quality, c.DurationMs, c.Role = hash, cliplibrary.ClipRelPath(hash, ".mp4"), quality, durationMs, clips.Commercial
		c.Placement = clips.PlacementBreakBody
		return c
	}
	spot := identitySpot(1)
	cut := creative.Fingerprint{DurationMs: 15_000, Video: append(append([]uint64(nil), spot.Video[:30]...), spot.Video[60:90]...)}
	st := &identityMemStore{
		clips: []IdentityClip{
			clip(soft, "240p", 30_000, t0),
			clip(sharp, "480p", 30_000, t0.Add(time.Hour)),
			clip(cutDown, "480p", 15_000, t0.Add(2*time.Hour)),
		},
		fingerprints: map[string]creative.Fingerprint{},
		variants:     map[string][]string{},
	}
	fakeTools := &identityTools{byHash: map[string]creative.Fingerprint{sharp: spot, soft: spot, cutDown: cut}}

	res, err := NewJob(st, fakeTools, dir, func() float64 { return -23 }, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fingerprinted != 3 || res.Merged != 1 || res.Linked != 1 {
		t.Fatalf("pass = %+v, want 3 fingerprinted, 1 merged, 1 linked", res)
	}
	want := []InferiorCopy{{ClipHash: soft, SourceHash: sourceHash, SurvivorHash: sharp, DecidedAt: t0}}
	if !reflect.DeepEqual(st.merges, want) {
		t.Fatalf("merges = %+v, want %+v", st.merges, want)
	}
	if len(st.links) != 1 || !reflect.DeepEqual(st.variants[cutDown], []string{sharp}) {
		t.Fatalf("links = %v, want the cut-down linked to the survivor once", st.links)
	}
	for _, gone := range []string{softFull, tools.SidecarPathFor(softFull), masterFull} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("the inferior copy's file %s survived the merge: %v", gone, err)
		}
	}
	for _, kept := range []string{sharp, cutDown} {
		if _, err := os.Stat(filepath.Join(dir, cliplibrary.ClipRelPath(kept, ".mp4"))); err != nil {
			t.Errorf("an ad that stays lost its file: %v", err)
		}
	}

	// The next pass reuses every fingerprint and finds nothing new to do.
	fakeTools.calls = 0
	st.clips = st.clips[1:]
	res, err = NewJob(st, fakeTools, dir, func() float64 { return -23 }, time.Now, nil).Run(ctx)
	if err != nil || fakeTools.calls != 0 || res.Merged != 0 || res.Linked != 0 {
		t.Fatalf("second pass = %+v (decodes %d, err %v), want cached fingerprints and no merge", res, fakeTools.calls, err)
	}
}
