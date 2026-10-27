package filler

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/creative"
)

type identityMemStore struct {
	clips        []IdentityClip
	fingerprints map[string]creative.Fingerprint
	variants     map[string][]string
	merges       []InferiorCopy
	links        [][2]string
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
	return nil
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
	var clips []IdentityClip
	for i, spec := range []struct {
		fp      creative.Fingerprint
		quality string
	}{{a, "480p"}, {b, "240p"}, {c, "240p"}} {
		hash := writeContentAddressedClip(t, dir, []byte{byte(i), 'x'}, ".mp4")
		hashes[hash] = spec.fp
		ic := IdentityClip{CreatedAt: t0.Add(time.Duration(i) * time.Hour)}
		ic.Hash, ic.Path, ic.Quality, ic.DurationMs, ic.Kind = hash, ClipRelPath(hash, ".mp4"), spec.quality, 30_000, Commercial
		ic.Placement = PlacementBreakBody
		clips = append(clips, ic)
	}
	st := &identityMemStore{clips: clips, fingerprints: map[string]creative.Fingerprint{}, variants: map[string][]string{}}
	res, err := NewCreativeIdentity(st, &identityTools{byHash: hashes}, dir, nil, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged != 1 || len(st.merges) != 1 || st.merges[0].ClipHash != clips[1].Hash || st.merges[0].SurvivorHash != clips[0].Hash {
		t.Fatalf("merges = %+v, want only B folded into A", st.merges)
	}
	if !reflect.DeepEqual(st.variants[clips[2].Hash], []string{clips[0].Hash}) {
		t.Fatalf("variants = %v, want C linked to A", st.variants)
	}
}

// One pass over a catalog holding an ad twice and a cut-down of it: the sharper copy survives even
// though the other arrived first, the other is merged with its source identity and its own files
// (media, sidecar, source master) deleted, and the cut-down is linked to the survivor.
func TestCreativeIdentity_KeepsTheBestCopyAndLinksTheCutDown(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()

	sharp := writeContentAddressedClip(t, dir, []byte("the 480p copy"), ".mp4")
	soft := writeContentAddressedClip(t, dir, []byte("the 240p copy, admitted first"), ".mp4")
	cutDown := writeContentAddressedClip(t, dir, []byte("the 15 s cut-down"), ".mp4")
	softFull := filepath.Join(dir, ClipRelPath(soft, ".mp4"))

	sourceFile := filepath.Join(dir, "soft-source.mkv")
	if err := os.WriteFile(sourceFile, []byte("the soft copy's downloaded bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceHash, err := ClipID(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	master, err := preserveSourceMaster(ctx, dir, sourceFile, sourceHash, SidecarTags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarTags(softFull, SidecarTags{MediaAssets: &MediaAssetManifest{
		Version: mediaAssetManifestVersion, SourceMaster: master,
	}}, false); err != nil {
		t.Fatal(err)
	}
	masterFull := filepath.Join(dir, filepath.FromSlash(master.Path))

	clip := func(hash, quality string, durationMs int64, created time.Time) IdentityClip {
		c := IdentityClip{CreatedAt: created}
		c.Hash, c.Path, c.Quality, c.DurationMs, c.Kind = hash, ClipRelPath(hash, ".mp4"), quality, durationMs, Commercial
		c.Placement = PlacementBreakBody
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
	tools := &identityTools{byHash: map[string]creative.Fingerprint{sharp: spot, soft: spot, cutDown: cut}}

	res, err := NewCreativeIdentity(st, tools, dir, func() float64 { return -23 }, func() time.Time { return t0 }, nil).Run(ctx)
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
	for _, gone := range []string{softFull, sidecarPathFor(softFull), masterFull} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("the inferior copy's file %s survived the merge: %v", gone, err)
		}
	}
	for _, kept := range []string{sharp, cutDown} {
		if _, err := os.Stat(filepath.Join(dir, ClipRelPath(kept, ".mp4"))); err != nil {
			t.Errorf("an ad that stays lost its file: %v", err)
		}
	}

	// The next pass reuses every fingerprint and finds nothing new to do.
	tools.calls = 0
	st.clips = st.clips[1:]
	res, err = NewCreativeIdentity(st, tools, dir, func() float64 { return -23 }, time.Now, nil).Run(ctx)
	if err != nil || tools.calls != 0 || res.Merged != 0 || res.Linked != 0 {
		t.Fatalf("second pass = %+v (decodes %d, err %v), want cached fingerprints and no merge", res, tools.calls, err)
	}
}
