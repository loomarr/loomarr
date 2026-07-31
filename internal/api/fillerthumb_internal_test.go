package api

import (
	"path/filepath"
	"testing"
)

// safeThumbPath is tested IN-PACKAGE, and that is a deliberate choice rather than a shortcut.
//
// ⚠ **The HTTP-level traversal test cannot reach this function, and I proved that the hard way.**
// The first version of `TestServeFillerThumb_RefusesTraversal` sent `../../secret.txt` and passed
// — then kept passing with the containment check DELETED. Two layers strip the attack before any
// handler runs: net/http's client rewrites the URL before sending, and Go 1.22+ ServeMux refuses
// to decode `%2f` into a separator when matching a `{path...}` wildcard, so an encoded attempt is
// rejected by the mux with its own 404 rather than reaching us.
//
// That makes this function defense-in-depth against a case the stdlib currently prevents — which
// is worth keeping (a future route mounted differently, a proxy that pre-decodes, a caller that
// passes a stored path straight in) but means an end-to-end test of it is theatre. So the
// guarantee is asserted where it is real: on the function.
func TestSafeThumbPath(t *testing.T) {
	const dir = "/srv/clips"
	base := filepath.Join(dir, "/.loomarr-thumbs")

	t.Run("allows nested paths, which are the normal case", func(t *testing.T) {
		// V28 preserves directory structure rather than flattening — two clips named
		// intro.mp4 in different era folders are different clips — so separators in the
		// input are legitimate and this cannot be a filename-only check.
		got, ok := safeThumbPath(dir, "80s/toys/intro.jpg")
		if !ok {
			t.Fatal("rejected a legitimate nested thumbnail path")
		}
		if want := filepath.Join(base, "80s/toys/intro.jpg"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("allows interior dot-dot that stays inside", func(t *testing.T) {
		// Resolves back into the tree, so it is safe — rejecting it would be a
		// string-matching answer to a filesystem question.
		got, ok := safeThumbPath(dir, "sub/../80s/toys/intro.jpg")
		if !ok {
			t.Fatal("rejected a path that resolves inside the thumbnail directory")
		}
		if want := filepath.Join(base, "80s/toys/intro.jpg"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("refuses escapes", func(t *testing.T) {
		for _, rel := range []string{
			"../../secret.txt",
			"a/../../../secret.txt",
			// Right prefix BEFORE cleaning, wrong one after — what a raw string-prefix
			// check misses, and the reason this uses filepath.Rel on absolute forms.
			"sub/../../../secret.txt",
			"../.loomarr-thumbs-evil/x.jpg",
		} {
			if _, ok := safeThumbPath(dir, rel); ok {
				t.Errorf("%q was accepted — it resolves outside the thumbnail directory", rel)
			}
		}
	})

	t.Run("refuses a NUL byte", func(t *testing.T) {
		// A NUL truncates the path at the syscall boundary on some platforms, so
		// `intro.jpg\x00.txt` can open a different file than it appears to name.
		if _, ok := safeThumbPath(dir, "intro.jpg\x00.txt"); ok {
			t.Error("accepted a path containing NUL")
		}
	})
}
