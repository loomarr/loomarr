package filler

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
)

// Info-JSON sidecars (§10). Both ingest paths write one next to every clip they
// download — yt-dlp via `--write-info-json`, and the Archive.org walker by hand in
// the same shape — precisely so AI tagging has real text to work with instead of a
// filename. Nothing read them until now: `Classify` was being handed `Clip.Source`,
// which is a PROVENANCE enum ("tunarr-local"), so every clip's prompt said
// "Source description: tunarr-local". That is worse than no description — it is a
// misleading signal fed to a classifier.
//
// Two different concepts had collapsed into one `string` field, which is why the
// compiler never caught it:
//   - Clip.Source     — provenance. Where the clip came from. An enum, for the catalog.
//   - sidecar text    — description. What the clip is ABOUT. Free text, for the LLM.

// sidecarInfo is the subset of the info-JSON we use. yt-dlp writes a large object;
// Archive.org's writer emits the same field names for the ones that matter. Unknown
// fields are ignored, so a yt-dlp version bump can't break parsing.
type sidecarInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	// yt-dlp records the uploader/channel; Archive.org records the collection. Either
	// is a useful hint about what KIND of clip this is (a toy-ad channel vs a news reel).
	Uploader string `json:"uploader"`
	Channel  string `json:"channel"`
	// Upload date ("20240131" from yt-dlp) is a weak era hint — weak because it is when
	// the clip was UPLOADED, not when it aired. Deliberately not used for Era: a 1985
	// commercial uploaded in 2024 would be tagged 2024, which is exactly the kind of
	// confidently-wrong metadata §10's grounding pass exists to keep out.
	UploadDate string `json:"upload_date"`
	// License is the source's declared licence URL (V33). ⚠ NOT a text signal — it never
	// reaches the tagger's prompt, because "CC BY-NC-SA 4.0" says nothing about whether a
	// clip is a cereal advert. It is a catalog fact, read by SidecarLicense instead.
	License string `json:"license"`
}

// sidecarPathFor returns the info-JSON path for a media file: "clip.mp4" →
// "clip.info.json". Mirrors how both writers name them.
func sidecarPathFor(mediaPath string) string {
	return strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)) + ".info.json"
}

// SidecarText reads the info-JSON beside a clip and renders the text signals as a
// short description for the tagger. Returns "" when there is no sidecar, it does not
// parse, or it carries nothing useful — an empty string is the honest answer, and
// tagUserPrompt already omits the line entirely rather than printing a blank one.
//
// Deliberately lossy: the model gets a few lines of real prose, not a JSON dump. A
// dump would bury the signal and burn tokens on fields (formats, thumbnails, chapter
// markers) that say nothing about what the clip IS.
func SidecarText(fsys fs.FS, mediaPath string) string {
	raw, err := fs.ReadFile(fsys, sidecarPathFor(mediaPath))
	if err != nil {
		return "" // no sidecar is normal — drop-folder clips never had one
	}
	var info sidecarInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return "" // malformed sidecar degrades to filename-only, never fails the tag
	}
	return info.text()
}

// text renders the parsed sidecar into the prompt fragment. Kept separate from the
// file read so the rendering rules are testable without a filesystem.
func (s sidecarInfo) text() string {
	var parts []string
	if t := strings.TrimSpace(s.Title); t != "" {
		parts = append(parts, t)
	}
	// Uploader/channel, whichever the writer filled in. Both being present is normal
	// for yt-dlp (they're often equal), so don't print it twice.
	by := strings.TrimSpace(s.Uploader)
	if by == "" {
		by = strings.TrimSpace(s.Channel)
	}
	if by != "" && !strings.EqualFold(by, strings.TrimSpace(s.Title)) {
		parts = append(parts, "from "+by)
	}
	// Descriptions can run to hundreds of lines of boilerplate (subscribe links,
	// timestamps). Take the leading prose — the part that actually describes the clip —
	// and cap it so one verbose uploader can't dominate the prompt.
	if d := firstProse(s.Description); d != "" {
		parts = append(parts, d)
	}
	return strings.Join(parts, " · ")
}

// maxDescriptionRunes bounds the description contribution. Generous enough for a real
// two-sentence summary, small enough that a wall of boilerplate can't crowd out the
// filename and the system prompt.
const maxDescriptionRunes = 280

// firstProse takes the leading non-empty lines of a description, stopping at the
// first line that looks like boilerplate rather than prose, and truncates on a rune
// boundary so multi-byte text is never cut mid-character.
func firstProse(desc string) string {
	var b strings.Builder
	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if b.Len() > 0 {
				break // blank line after real content ends the lead paragraph
			}
			continue
		}
		if isBoilerplate(line) {
			break
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(line)
		if len([]rune(b.String())) >= maxDescriptionRunes {
			break
		}
	}
	out := []rune(strings.TrimSpace(b.String()))
	if len(out) > maxDescriptionRunes {
		return strings.TrimSpace(string(out[:maxDescriptionRunes])) + "…"
	}
	return string(out)
}

// isBoilerplate spots the lines that pad uploader descriptions and carry no signal
// about the clip's content. Narrow on purpose: a false positive only truncates the
// description early, but being too eager would drop real prose.
func isBoilerplate(line string) bool {
	l := strings.ToLower(line)
	switch {
	case strings.HasPrefix(l, "http://"), strings.HasPrefix(l, "https://"):
		return true
	case strings.HasPrefix(l, "subscribe"), strings.HasPrefix(l, "follow "):
		return true
	case strings.Contains(l, "patreon.com"), strings.Contains(l, "ko-fi.com"):
		return true
	}
	return false
}

// SidecarLicense reads the licence URL a source declared for a clip (V33). Returns "" when
// there is no sidecar, it does not parse, or the source declared none.
//
// ⚠ **Empty means UNKNOWN, never "public domain".** About 92% of Archive items carry no
// licence at all (667 of 8362 in `classic_tv_commercials` — measured during the 2026-07-31
// fixture capture), so absence is the common case and says nothing about permission. Callers
// render "unknown", never a reassuring default.
//
// ⚠ Separate from SidecarText, deliberately. That function builds PROSE for the tagger, and a
// licence is a catalog fact — "CC BY-NC-SA 4.0" tells a model nothing about whether a clip is a
// cereal advert, and it would burn prompt tokens. Worse, it would not survive the trip:
// `isBoilerplate` drops any line starting with http(s)://, which is every licence URL.
func SidecarLicense(fsys fs.FS, mediaPath string) string {
	raw, err := fs.ReadFile(fsys, sidecarPathFor(mediaPath))
	if err != nil {
		return "" // no sidecar is normal — drop-folder clips never had one
	}
	var info sidecarInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ""
	}
	return strings.TrimSpace(info.License)
}
