package playout

import "os"

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
