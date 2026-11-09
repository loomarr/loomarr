// Package filler is the commercials & filler domain (design §10): the clip
// catalog model; pod assembly lives in filler/pod. Filler is a PARALLEL universe to provisioning
// (§3–§7) — clips are not titles (not in TMDB, no acquisition loop). Their
// identity is the clip's sparse content HASH (§10 V38c; see Clip.Hash) and their
// duration comes from Loomarr's own ffprobe scan.
//
// ⚠ **Loomarr discovers its own clips.** Tunarr does not: it is optional, and when
// present it only supplies program uuids for clips it already knows (see
// TunarrClipSource) — an install running internal playout with no Tunarr has a full
// catalog. The MEDIA SERVER, however, IS one of the scan sources: `library` is a
// source kind alongside `folder`, `youtube` and `archive`, read via
// library.ListFillerClips. This comment claimed "the media server is not in the
// filler path" until 2026-08-10, which had not been true since sources became
// pluggable.
//
// Pod assembly (filler/pod) is pure and SEEDED-DETERMINISTIC; see that package.
// This domain only decides *what plays in the breaks*.
package filler

// ClipQuery is filler's view of a clip filter — deliberately tiny, because its callers want
// "everything currently in the catalog" and do their own filtering.
//
// ⚠ It stays small on purpose. `language = ”` and `transcript = ”` are not concepts the store
// should learn: they are the "not yet checked" sentinels of individual pipeline rungs, and pushing
// them down would make the store's filter grow a clause per stage.
type ClipQuery struct {
	// IncludeHeld covers clips still awaiting review. A held clip is a fine candidate for most
	// work: knowing a clip's language, transcript or tags BEFORE a human looks is strictly more
	// useful than after.
	IncludeHeld bool
}
