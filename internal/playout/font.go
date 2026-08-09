package playout

import (
	"os"
	"sync"
)

// Font discovery for the test/offline card's `drawtext` filter.
//
// drawtext fails at INIT when given a fontfile that does not exist — not at draw time, so
// the whole encode dies rather than rendering an unlabelled card. That makes "did we find
// a real file?" the only question worth answering here, and why FindFont stats each
// candidate rather than returning a plausible-looking path.
//
// The image installs `fonts-dejavu-core` (§16) so the first candidate normally hits. The
// rest of the list covers a developer running the binary on a host rather than in the
// image — a distro-specific layout should degrade to an unlabelled card, never to a dead
// channel.
var fontCandidates = []string{
	// Debian/Ubuntu — what the image ships.
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
	// Arch and several others put TTFs a level up.
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/TTF/LiberationSans-Regular.ttf",
	// Fedora/RHEL.
	"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
	"/usr/share/fonts/liberation-sans/LiberationSans-Regular.ttf",
	// macOS, for a developer running tests outside a container.
	"/System/Library/Fonts/Supplemental/Arial.ttf",
}

// FontPathEnv lets an operator point at a font the list does not know about — a custom
// image, or simply a preference. Deliberately NOT a registry setting (§15): the font is a
// property of the filesystem the process runs on, like the ffmpeg path, not something the
// app manages or that would round-trip through a database.
const FontPathEnv = "PLAYOUT_FONT_PATH"

// FindFont returns a usable font file, or "" when there is none. "" is a supported
// answer, not a failure: the card renders as a plain colour field (see drawTextFilter).
func FindFont() string {
	if p := os.Getenv(FontPathEnv); p != "" && fileExists(p) {
		return p
	}
	for _, p := range fontCandidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// CardFontFor returns the font a card should be labelled with, or "" when this host cannot
// render text at all — bound to an ffmpeg path the same way FFprobeAudioNextTo is, and for
// the same reason.
//
// ⚠ A FONT FILE IS NOT ENOUGH. `drawtext` is a compile-time option (libfreetype, plus
// libharfbuzz on ffmpeg 8), and a build without it rejects the filter at graph-init with
// "Filter not found" — the encode exits 8 and the channel is DEAD, which is exactly the
// outcome the comment above promises never to cause. Homebrew's `ffmpeg` bottle is the case
// that found this: macOS has Arial.ttf, so the font check passed, and every card died.
// FindFont answers "is there a font?"; only the build answers "can it draw one?".
//
// So this asks the binary, the same way listEncoders does — the build itself is the only
// honest signal, and it costs one `-filters` exec that is memoised for the process. A probe
// that cannot run resolves to "" (unlabelled card), never to an assumed yes: the failure
// this exists to prevent is a dead channel, and an unlabelled card is the safe direction.
func CardFontFor(ffmpegPath string) func() string {
	var (
		once sync.Once
		font string
	)
	return func() string {
		once.Do(func() {
			if hasDrawText(ffmpegPath) {
				font = FindFont()
			}
		})
		return font
	}
}

// hasDrawText reports whether this ffmpeg build carries the drawtext filter.
//
// A thin caller of filters.go's hasFilter — this file discovered the "a filter is a per-build
// fact" rule, and filters.go now owns it for every optional filter. Kept as a named function
// because "can this build draw text?" is the question CardFontFor asks, and spelling it out here
// keeps that reasoning next to the font list it guards.
func hasDrawText(ffmpegPath string) bool { return hasFilter(ffmpegPath, "drawtext") }

// parseHasDrawText is the pure half, retained so the existing column-matching tests keep their
// subject. The matching itself lives in parseHasFilter.
func parseHasDrawText(raw []byte) bool { return parseHasFilter(raw, "drawtext") }
