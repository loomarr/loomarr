package filler

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/creative"
	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/mediatools"
)

// identityFingerprintBudget bounds how long one pass spends decoding. A catalog that needs more
// is fingerprinted over several passes; comparison only ever uses what is already measured.
const identityFingerprintBudget = 40 * time.Second

// InferiorCopy is one copy of an ad folded into the better copy that survives (#2033).
type InferiorCopy struct {
	ClipHash, SourceHash, SurvivorHash string
	DecidedAt                          time.Time
}

// IdentityStore is the slice of the store creative identity needs.
type IdentityStore interface {
	// ListIdentityClips is every catalog clip, composites, held and removed clips included, with
	// its admission time. Only published clips are compared, but every sidecar guards shared
	// assets: a removed clip can be restored.
	ListIdentityClips(ctx context.Context) ([]IdentityClip, error)
	ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error)
	UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error
	ListClipVariants(ctx context.Context) (map[string][]string, error)
	LinkClipVariants(ctx context.Context, a, b string) error
	MergeInferiorCopy(ctx context.Context, c InferiorCopy) error
	// ListWantedCopies is every retired defective copy still waiting for a better one (#2036).
	ListWantedCopies(ctx context.Context) ([]WantedCopy, error)
}

// IdentityClip is a catalog clip and when it was admitted, the last tie-break between copies.
type IdentityClip struct {
	StoreClip
	CreatedAt time.Time
	// Defects is the clip's current defect assessment; nil until the floor has measured it.
	Defects *defects.Assessment
}

// WantedCopy is an ad whose only copy failed the defect floor and was retired. Its row keeps the
// fingerprint and the airings a cleaner copy inherits.
type WantedCopy struct {
	ClipHash, Path string
}

// IdentityResult reports one pass.
type IdentityResult struct {
	Fingerprinted int // clips measured this pass
	Merged        int // inferior copies folded into their survivor
	Upgraded      int // retired defective copies a cleaner copy of the same ad replaced
	Linked        int // new variant links
	Pending       int // published clips still waiting for a fingerprint
}

// CreativeIdentity makes the catalog hold each ad once (#2033). Every pass fingerprints published
// clips that lack one, folds every copy of an ad into its best copy, deletes the losers' files,
// and links variants. It is the one writer of merges and links, for new arrivals and for a
// catalog that predates it alike: a clip's turn comes when it has a fingerprint.
type CreativeIdentity struct {
	store   IdentityStore
	tools   mediatools.MediaTools
	clipDir string
	// targetLUFS is the live air-time target; a copy closer to it needs less correction.
	targetLUFS func() float64
	now        func() time.Time
	log        *slog.Logger
}

func NewCreativeIdentity(store IdentityStore, tools mediatools.MediaTools, clipDir string,
	targetLUFS func() float64, now func() time.Time, log *slog.Logger) *CreativeIdentity {
	return &CreativeIdentity{store: store, tools: tools, clipDir: clipDir, targetLUFS: targetLUFS, now: now, log: log}
}

// Run is one pass. A failure on one clip or pair is logged and left for the next pass.
func (ci *CreativeIdentity) Run(ctx context.Context) (IdentityResult, error) {
	var res IdentityResult
	all, err := ci.store.ListIdentityClips(ctx)
	if err != nil {
		return res, err
	}
	cached, err := ci.store.ListClipFingerprints(ctx, creative.Algorithm)
	if err != nil {
		// Corrupt rows are omitted; the valid rest is still usable and the rest is remeasured.
		ci.warn("identity: some cached fingerprints were unreadable", "err", err)
	}
	published := make([]IdentityClip, 0, len(all))
	for _, c := range all {
		if identityCandidate(c.Clip) {
			published = append(published, c)
		}
	}
	// Oldest first, so a backlog is measured in admission order and the result is deterministic.
	slices.SortFunc(published, func(a, b IdentityClip) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Hash, b.Hash))
	})
	deadline := ci.now().Add(identityFingerprintBudget)
	measured := make([]IdentityClip, 0, len(published))
	fps := make(map[string]creative.Fingerprint, len(published))
	for _, c := range published {
		if fp, ok := cached[c.Hash]; ok {
			fps[c.Hash] = fp
			measured = append(measured, c)
			continue
		}
		if ctx.Err() != nil || ci.now().After(deadline) {
			res.Pending++
			continue
		}
		full, err := ClipPath(ci.clipDir, c.Path, filepath.Ext(c.Path))
		if err != nil {
			continue
		}
		fp, err := ci.tools.Fingerprint(ctx, full, 0, c.DurationMs)
		if err != nil || len(fp.Video) == 0 {
			ci.warn("identity: clip could not be fingerprinted", "clip", c.Hash, "err", err)
			continue
		}
		if err := ci.store.UpsertClipFingerprint(ctx, c.Hash, creative.Algorithm, fp); err != nil {
			return res, err
		}
		res.Fingerprinted++
		fps[c.Hash] = fp
		measured = append(measured, c)
	}

	// Copies group transitively, but a copy is merged only into a survivor it matches directly (see
	// below): Same tolerates a few seconds of difference, so a chain of matches can span more.
	group := newUnionFind(len(measured))
	same := map[[2]int]bool{}
	var variants [][2]int
	for i := range measured {
		// Every pair is compared each pass; the job stops cleanly when its lease ends.
		if err := ctx.Err(); err != nil {
			return res, err
		}
		for j := i + 1; j < len(measured); j++ {
			switch creative.Compare(fps[measured[i].Hash], fps[measured[j].Hash]) {
			case creative.Same:
				group.union(i, j)
				same[[2]int{i, j}] = true
			case creative.Variant:
				variants = append(variants, [2]int{i, j})
			}
		}
	}
	members := map[int][]int{}
	for i := range measured {
		members[group.find(i)] = append(members[group.find(i)], i)
	}
	paths := make([]string, len(all))
	for i, c := range all {
		paths[i] = c.Path
	}
	referenced := referencedAssets(ci.clipDir, paths)
	survivorOf := make([]int, len(measured))
	for i := range survivorOf {
		survivorOf[i] = i
	}
	for _, copies := range members {
		if len(copies) < 2 {
			continue
		}
		best := slices.MinFunc(copies, func(a, b int) int { return ci.compareCopies(measured[a], measured[b]) })
		for _, loser := range copies {
			if loser == best || !same[[2]int{min(loser, best), max(loser, best)}] {
				// Only reachable through another copy, e.g. a trimmed edit matching the full one
				// while the survivor is trimmed the other way. It stays its own clip.
				continue
			}
			survivorOf[loser] = best
			if err := ci.merge(ctx, measured[loser], measured[best], referenced); err != nil {
				ci.warn("identity: copy could not be merged", "clip", measured[loser].Hash,
					"survivor", measured[best].Hash, "err", err)
				survivorOf[loser] = loser
				continue
			}
			res.Merged++
		}
	}
	if err := ci.upgradeWanted(ctx, cached, measured, fps, survivorOf, referenced, &res); err != nil {
		return res, err
	}

	existing, err := ci.store.ListClipVariants(ctx)
	if err != nil {
		return res, err
	}
	for _, pair := range variants {
		a, b := measured[survivorOf[pair[0]]].Hash, measured[survivorOf[pair[1]]].Hash
		if a == b || slices.Contains(existing[a], b) {
			continue
		}
		if err := ci.store.LinkClipVariants(ctx, a, b); err != nil {
			return res, err
		}
		existing[a], existing[b] = append(existing[a], b), append(existing[b], a)
		res.Linked++
	}
	if ci.log != nil && (res.Merged > 0 || res.Linked > 0 || res.Upgraded > 0) {
		ci.log.Info("identity: catalog holds each ad once",
			"merged", res.Merged, "upgraded", res.Upgraded, "linked", res.Linked, "fingerprinted", res.Fingerprinted,
			"pending", res.Pending)
	}
	return res, nil
}

// upgradeWanted folds each retired defective copy into the best published copy of the same ad
// (#2036). The merge moves the retired copy's airings, plays and pins to it, so the upgrade airs in
// the old copy's place in rotation, and the ad leaves the wanted list. Every source's arrivals meet
// this check once they are published and fingerprinted, here.
func (ci *CreativeIdentity) upgradeWanted(ctx context.Context, cached map[string]creative.Fingerprint,
	measured []IdentityClip, fps map[string]creative.Fingerprint, survivorOf []int, referenced map[string]int,
	res *IdentityResult) error {
	wanted, err := ci.store.ListWantedCopies(ctx)
	if err != nil {
		return err
	}
	for _, w := range wanted {
		fp, ok := cached[w.ClipHash]
		if !ok {
			continue // retired before it could be fingerprinted; nothing can match it
		}
		best := -1
		for i, c := range measured {
			if survivorOf[i] != i || creative.Compare(fp, fps[c.Hash]) != creative.Same {
				continue
			}
			if c.Defects != nil && c.Defects.Outcome == defects.Fail {
				continue // no upgrade: the floor retires it in turn
			}
			if best < 0 || ci.compareCopies(c, measured[best]) < 0 {
				best = i
			}
		}
		if best < 0 {
			continue
		}
		retired := IdentityClip{}
		retired.Hash, retired.Path = w.ClipHash, w.Path
		if err := ci.merge(ctx, retired, measured[best], referenced); err != nil {
			ci.warn("identity: a retired copy could not be replaced by its upgrade", "clip", w.ClipHash,
				"survivor", measured[best].Hash, "err", err)
			continue
		}
		res.Upgraded++
	}
	return nil
}

// identityCandidate is a clip that airs as an ad of its own: published, not a compilation.
func identityCandidate(c Clip) bool {
	return !c.IsComposite && !c.Held && c.RemovedAt.IsZero() && c.Placement != PlacementNotPlayable
}

// compareCopies orders two copies of one ad best first: the cleaner copy, with fewer checks near
// or past their bar (#2036), then the higher resolution, then the copy needing less air-time
// loudness correction (within 1 dB is a tie), then the more complete edit, then the earlier
// admission. An unmeasured copy ranks after every measured one on the first rung.
func (ci *CreativeIdentity) compareCopies(a, b IdentityClip) int {
	if d := cmp.Compare(defectsFound(a), defectsFound(b)); d != 0 {
		return d
	}
	if d := cmp.Compare(qualityRank(b.Quality), qualityRank(a.Quality)); d != 0 {
		return d
	}
	if ga, gb := ci.correctionNeeded(a), ci.correctionNeeded(b); math.Abs(ga-gb) >= 1 {
		return cmp.Compare(ga, gb)
	}
	return cmp.Or(cmp.Compare(b.DurationMs, a.DurationMs), a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Hash, b.Hash))
}

// defectsFound counts a copy's findings short of a clean pass, a failing one counting for more
// than any number of borderline ones. An unmeasured copy counts as worst.
func defectsFound(c IdentityClip) int {
	if c.Defects == nil {
		return math.MaxInt
	}
	n := 0
	for _, f := range c.Defects.Findings {
		switch f.Outcome {
		case defects.Fail:
			n += 1000
		case defects.Borderline:
			n++
		}
	}
	return n
}

// correctionNeeded is how far a copy's measured playback loudness sits from the target, in dB. An
// unmeasured copy ranks last on this rung.
func (ci *CreativeIdentity) correctionNeeded(c IdentityClip) float64 {
	full, err := ClipPath(ci.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil || ci.targetLUFS == nil {
		return math.Inf(1)
	}
	tags, ok := ReadSidecarTags(full)
	if !ok || tags.MediaAssets == nil || tags.MediaAssets.Playback == nil {
		return math.Inf(1)
	}
	measured := tags.MediaAssets.Playback.QC.Loudness
	if !measured.Available || math.IsNaN(measured.IntegratedLUFS) || math.IsInf(measured.IntegratedLUFS, 0) {
		return math.Inf(1)
	}
	return math.Abs(ci.targetLUFS() - measured.IntegratedLUFS)
}

// qualityRank reads the height out of a quality label: "720p" is 720, "4K" 2160, unknown 0.
func qualityRank(q string) int {
	if strings.EqualFold(q, "4K") {
		return 2160
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(q), "p"))
	if err != nil {
		return 0
	}
	return n
}

// merge folds loser into survivor durably, then deletes the loser's own files. The order is the
// correctness story, as in SplitSweeper.retire: once the merge commits, Sync never catalogues the
// loser again, so a failed unlink leaves only bytes, never a second copy on air.
func (ci *CreativeIdentity) merge(ctx context.Context, loser, survivor IdentityClip, referenced map[string]int) error {
	full, err := ClipPath(ci.clipDir, loser.Path, filepath.Ext(loser.Path))
	if err != nil {
		return fmt.Errorf("loser path: %w", err)
	}
	tags, _ := ReadSidecarTags(full)
	source := loser.Hash
	if tags.MediaAssets != nil && tags.MediaAssets.SourceMaster.ClipHash != "" {
		source = tags.MediaAssets.SourceMaster.ClipHash
	}
	if err := ci.store.MergeInferiorCopy(ctx, InferiorCopy{
		ClipHash: loser.Hash, SourceHash: source, SurvivorHash: survivor.Hash, DecidedAt: ci.now().UTC(),
	}); err != nil {
		return err
	}
	for f, err := range removeOwnFiles(ci.clipDir, full, tags, referenced) {
		ci.warn("identity: an inferior copy's file could not be removed; it is already off the catalog",
			"clip", loser.Hash, "file", f, "err", err)
	}
	return nil
}

// removeOwnFiles deletes a clip that is already off the catalog from disk: its playback file, its
// sidecar, and the source master and evidence copy its sidecar names, unless another clip's sidecar
// names them too. It reports each file it could not remove.
func removeOwnFiles(clipDir, full string, tags SidecarTags, referenced map[string]int) map[string]error {
	files := []string{full, sidecarPathFor(full)}
	if m := tags.MediaAssets; m != nil {
		// An asset another clip's sidecar also names is shared; it stays. The count drops either
		// way, so the last of two copies removed in one pass takes it.
		for _, asset := range []string{m.SourceMaster.Path, derivativePath(m.Evidence)} {
			if asset == "" {
				continue
			}
			if referenced[asset] == 1 {
				files = append(files, filepath.Join(clipDir, filepath.FromSlash(asset)))
			}
			referenced[asset]--
		}
	}
	failed := map[string]error{}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			failed[f] = err
		}
	}
	return failed
}

func derivativePath(d *MediaDerivativeLineage) string {
	if d == nil {
		return ""
	}
	return d.Asset.Path
}

// referencedAssets counts how many clips' sidecars name each media asset, across the whole catalog.
func referencedAssets(clipDir string, paths []string) map[string]int {
	out := map[string]int{}
	for _, path := range paths {
		full, err := ClipPath(clipDir, path, filepath.Ext(path))
		if err != nil {
			continue
		}
		tags, ok := ReadSidecarTags(full)
		if !ok || tags.MediaAssets == nil {
			continue
		}
		m := tags.MediaAssets
		for _, p := range []string{m.SourceMaster.Path, derivativePath(m.Evidence), derivativePath(m.Playback)} {
			if p != "" {
				out[p]++
			}
		}
	}
	return out
}

func (ci *CreativeIdentity) warn(msg string, args ...any) {
	if ci.log != nil {
		ci.log.Warn(msg, args...)
	}
}

type unionFind []int

func newUnionFind(n int) unionFind {
	u := make(unionFind, n)
	for i := range u {
		u[i] = i
	}
	return u
}

func (u unionFind) find(i int) int {
	for u[i] != i {
		u[i] = u[u[i]]
		i = u[i]
	}
	return i
}

func (u unionFind) union(a, b int) { u[u.find(a)] = u.find(b) }
