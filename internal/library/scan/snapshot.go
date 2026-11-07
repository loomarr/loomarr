package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
)

// Item kinds the scanner writes. Movie, series and episode match the media server's kinds, so the
// two origins of one title agree.
const (
	kindMovie   inventory.ItemKind = "movie"
	kindSeries  inventory.ItemKind = "series"
	kindEpisode inventory.ItemKind = "episode"
	kindExtra   inventory.ItemKind = "extra"
)

// Authority is the inventory authority of one scan root: every file under it is an origin of that
// authority, so a walk of the root marks exactly its own files unseen.
func Authority(root string) inventory.AuthorityID {
	digest := sha256.Sum256([]byte(root))
	return inventory.AuthorityID("scan:" + hex.EncodeToString(digest[:12]))
}

// fileOrigin is a media file's origin: its path within its root.
func fileOrigin(root, rel string) inventory.OriginKey {
	digest := sha256.Sum256([]byte(rel))
	return inventory.OriginKey{Authority: Authority(root), ExternalItemID: "file:" + hex.EncodeToString(digest[:16])}
}

// seriesOrigin is a series' origin within a root: its folder. Its ids are not part of the key, so
// learning them never forks the series, and finding it present needs no sidecar read.
func seriesOrigin(root, dir string) inventory.OriginKey {
	digest := sha256.Sum256([]byte(dir))
	return inventory.OriginKey{Authority: Authority(root), ExternalItemID: "series:dir:" + hex.EncodeToString(digest[:16])}
}

// revision is a file's inventory revision: its stat. A new size or mtime is new bytes.
func revision(st Stat) string { return fmt.Sprintf("stat:%d:%d", st.Size, st.ModifiedNs) }

// identityDigest digests everything that says what a file is, so a change to any of it, and
// nothing else, rewrites the file's inventory item.
func identityDigest(file File, m managed, embedded mp.Record, parent []inventory.ExternalID) string {
	payload, _ := json.Marshal(struct {
		Records  []mp.Record
		Series   *mp.Record
		Embedded mp.Record
		Sidecars map[string]Stat
		Parent   []inventory.ExternalID
		Extra    bool
	}{m.records, m.series, embedded, file.Sidecars, parent, file.Extra})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:16])
}

func externalIDs(picked []precedence.Picked[mp.ExternalID]) []inventory.ExternalID {
	out := make([]inventory.ExternalID, 0, len(picked))
	for _, p := range picked {
		out = append(out, inventory.ExternalID{Namespace: string(p.Value.Namespace), Value: p.Value.Value})
	}
	return out
}

// itemFacts projects resolved values into the inventory's item facts, with coverage for each
// family a source answered.
func itemFacts(r precedence.Resolved) (inventory.ItemFacts, map[string]inventory.Coverage) {
	f := inventory.ItemFacts{
		Name: r.Title.Value, OriginalTitle: r.OriginalTitle.Value, SortTitle: r.SortTitle.Value,
		Overview: r.Overview.Value, Genres: r.Genres.Value, ProductionYear: r.Year.Value,
		PremiereDate: r.PremiereDate.Value, OfficialRating: r.Certification.Value.Rating,
		RuntimeMillis: r.Runtime.Value.Milliseconds(),
	}
	if r.Network.Value != "" {
		f.Studios = []string{r.Network.Value}
	}
	if r.Numbering.Known() {
		f.SeasonNumber, f.EpisodeNumber, f.EpisodeEnd = r.Numbering.Value.Season, r.Numbering.Value.Episode, r.Numbering.Value.EpisodeEnd
	}
	for _, a := range r.Artwork {
		f.Artwork = append(f.Artwork, inventory.ArtworkRef{Kind: a.Value.Kind, Ref: a.Value.Ref})
	}
	for _, p := range r.Parent {
		f.Relationships = append(f.Relationships, inventory.Relationship{
			Kind: string(kindSeries), ExternalID: string(p.Value.Namespace) + ":" + p.Value.Value,
		})
	}
	coverage := map[string]inventory.Coverage{}
	if r.Title.Known() {
		coverage["descriptive"] = inventory.CoveragePresent
	}
	if r.Numbering.Known() {
		coverage["numbering"] = inventory.CoveragePresent
	}
	if r.Certification.Known() {
		coverage["certification"] = inventory.CoveragePresent
	}
	return f, coverage
}

// sourceSnapshot is a file as the inventory stores a source: its stat, plus its header facts once
// read.
func sourceSnapshot(file File, header *inventory.SourceFacts, at time.Time) inventory.SourceSnapshot {
	facts := inventory.SourceFacts{Protocol: "file", SizeBytes: file.Stat.Size, ModifiedUnixNano: file.Stat.ModifiedNs}
	coverage := map[string]inventory.Coverage{"sourceStat": inventory.CoveragePresent}
	if header != nil {
		facts.Container, facts.DurationMillis, facts.Bitrate, facts.Streams =
			header.Container, header.DurationMillis, header.Bitrate, header.Streams
		coverage["streams"] = inventory.CoveragePresent
		coverage["audioStreams"] = inventory.CoverageEmpty
		for _, s := range header.Streams {
			if s.Kind == inventory.StreamAudio {
				coverage["audioStreams"] = inventory.CoveragePresent
				break
			}
		}
	}
	return inventory.SourceSnapshot{
		ExternalSourceID: "file", Kind: inventory.SourceLocalFile, Revision: revision(file.Stat),
		Locator: inventory.Locator{Path: file.Path},
		Observation: inventory.Observation[inventory.SourceFacts]{
			SchemaVersion: 1, ObservedAt: at, Coverage: coverage, Facts: facts,
		},
	}
}

// fallbackName is a title from a file name when no source names the item.
func fallbackName(rel string) string {
	name := rel[strings.LastIndex(rel, "/")+1:]
	if dot := strings.LastIndex(name, "."); dot > 0 {
		name = name[:dot]
	}
	return strings.TrimSpace(embeddedID.ReplaceAllString(name, ""))
}
