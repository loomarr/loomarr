package filler_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

const testHash = "a3f9c2000000000000000000000000000000000000000000000000000000beef"

// A bare id resolves into its shard directories.
func TestClipPath_ShardsByTheHash(t *testing.T) {
	got, err := filler.ClipPath("/clips", testHash, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/clips", "a3", "f9", testHash+".mp4")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// A stored row's shard path resolves to the same place, so the row and the id cannot disagree
// about where a clip lives.
func TestClipPath_AcceptsAStoredShardPath(t *testing.T) {
	rel := filler.ClipRelPath(testHash, ".mp4")
	got, err := filler.ClipPath("/clips", filepath.ToSlash(rel), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("a3", "f9", testHash+".mp4")) {
		t.Errorf("got %s, want it under the shard dirs", got)
	}
}

// ⚠ THE security boundary. A clip id reaches here from the database and from API callers, so a
// crafted value must not be able to resolve outside the clip folder — playout would stream
// whatever it pointed at.
//
// Under hash identity the check is an ALLOW-LIST on the alphabet rather than path sanitising:
// nothing containing a separator, a dot or an encoding can match `^[0-9a-f]{64}$`.
func TestClipPath_RefusesAnythingThatIsNotAHash(t *testing.T) {
	for _, bad := range []string{
		"",
		"../../etc/passwd",
		"/etc/passwd",
		"a3/f9/../../../etc/passwd",
		"..%2f..%2fetc%2fpasswd",
		`..\..\windows\system32`,
		testHash + "/../../../etc/passwd",
		strings.ToUpper(testHash), // uppercase is not what ClipID produces
		testHash[:63],             // one character short
		"ads/coke.mp4",            // a pre-V38c path is no longer an id
	} {
		if got, err := filler.ClipPath("/clips", bad, ".mp4"); err == nil {
			t.Errorf("accepted %q → %s", bad, got)
		}
	}
}

// An empty clip folder is refused rather than resolving to a relative path off the working
// directory — which on a server is wherever the process happened to start.
func TestClipPath_RefusesAnEmptyClipFolder(t *testing.T) {
	if _, err := filler.ClipPath("", testHash, ".mp4"); err == nil {
		t.Error("accepted an empty clip folder")
	}
}
