package api

import (
	"path/filepath"
	"testing"
)

// The containment guard for the clip-media route (§10 V35), tested IN-PACKAGE.
//
// ⚠ This is where the traversal assertion lives, and the reason is written on
// `TestServeFillerMedia_TraversalNeverReturnsBytes`: an HTTP-level traversal test cannot fail,
// because net/http and Go's ServeMux both strip the attack before a handler sees it. Calling the
// function directly is the only way to prove it does anything.
func TestSafeFillerPath(t *testing.T) {
	const dir = "/srv/clips"

	t.Run("allows nested paths, which are the normal case", func(t *testing.T) {
		// A clip's identity IS its path relative to FILLER_DIR, structure preserved — two
		// clips named intro.mp4 in different era folders are different clips — so separators
		// in the input are legitimate and this cannot be a filename-only check.
		got, ok := safeFillerPath(dir, "80s/toys/intro.mp4")
		if !ok {
			t.Fatal("rejected a legitimate nested clip path")
		}
		if want := filepath.Join(dir, "80s/toys/intro.mp4"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("allows interior dot-dot that stays inside", func(t *testing.T) {
		// Resolves back into the tree, so it is safe — rejecting it would be a
		// string-matching answer to a filesystem question.
		got, ok := safeFillerPath(dir, "sub/../80s/intro.mp4")
		if !ok {
			t.Fatal("rejected a path that resolves inside the drop-folder")
		}
		if want := filepath.Join(dir, "80s/intro.mp4"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("refuses escapes", func(t *testing.T) {
		for _, rel := range []string{
			"../secret.txt",
			"../../etc/passwd",
			// Right prefix BEFORE cleaning, wrong one after — what a raw string-prefix
			// check misses, and the reason this uses filepath.Rel on absolute forms.
			"80s/../../../etc/passwd",
			// A NUL truncates the path at the syscall boundary on some platforms, so a
			// name that looks contained can open something else.
			"80s/intro.mp4\x00.jpg",
			"",
		} {
			if got, ok := safeFillerPath(dir, rel); ok {
				t.Errorf("%q was allowed, resolving to %q — it escapes the drop-folder", rel, got)
			}
		}
	})

	// An absolute-looking input is ROOTED into the drop-folder, not honoured — `filepath.Join`
	// treats a leading separator as just another separator. Asserted rather than listed as an
	// escape, because the first draft of this test assumed the opposite and failed: the answer
	// is "contained", which is the safe outcome, and pinning it is what stops someone
	// "fixing" the guard toward the dangerous reading.
	t.Run("roots an absolute path inside the folder rather than honouring it", func(t *testing.T) {
		got, ok := safeFillerPath(dir, "/etc/passwd")
		if !ok {
			t.Fatal("rejected outright; expected it to resolve INSIDE the drop-folder")
		}
		if want := filepath.Join(dir, "etc/passwd"); got != want {
			t.Errorf("got %q, want %q — an absolute input must not reach the real /etc", got, want)
		}
	})
}

// The extension allowlist is a security boundary, not a convenience: FILLER_DIR is
// operator-writable, so serving `mime.TypeByExtension` would let an .html dropped in that folder
// come back as text/html from Loomarr's own origin — stored XSS against the session.
func TestMediaTypes_RefusesAnythingThatCouldRenderAsADocument(t *testing.T) {
	for _, ext := range []string{".html", ".htm", ".svg", ".xml", ".js", ".txt", ".jpg", ".json", ""} {
		if ctype, ok := mediaTypes[ext]; ok {
			t.Errorf("%q is served as %q — the allowlist must hold media containers only", ext, ctype)
		}
	}
	// And the ones that must work, or the ▶ on every card is decorative.
	for _, ext := range []string{".mp4", ".mkv", ".webm", ".avi", ".mpg"} {
		if _, ok := mediaTypes[ext]; !ok {
			t.Errorf("%q is not served, but the scan accepts it", ext)
		}
	}
}
