// Package quarantine is where filler files Loomarr deletes on its own wait a day before their bytes
// go (#3855, "The goal is a 9" in #3838). A wrong delete can be undone by moving the file back, and
// the bytes still count toward the filler budget until they are gone, so a cull that frees space
// frees it a day later, never twice.
package quarantine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// DirName is the quarantine folder under the clip folder. The scan skips hidden folders, so nothing
// in it is catalogued.
var DirName = filepath.Join(split.MediaAssetRootName, "quarantine")

// Hold is how long a file waits in quarantine.
const Hold = 24 * time.Hour

// ErrOutsideRoot refuses a file that is not under the clip folder, or is already in quarantine.
var ErrOutsideRoot = errors.New("not a file under the clip folder")

// Bin moves files into quarantine and empties batches that have waited a day.
type Bin struct {
	root string
	now  func() time.Time
}

// New is the quarantine for the clip folder root.
func New(root string, now func() time.Time) Bin {
	return Bin{root: root, now: now}
}

// Put moves each file into this moment's batch, keeping its path under the clip folder so it can be
// put back by hand. A file already gone is not a failure. It reports each file it could not move;
// that file stays where it was.
func (b Bin) Put(files []string) map[string]error {
	failed := map[string]error{}
	batch := filepath.Join(b.root, DirName, strconv.FormatInt(b.now().Unix(), 10))
	for _, f := range files {
		rel, err := filepath.Rel(b.root, f)
		if err != nil || rel == "." || !filepath.IsLocal(rel) ||
			rel == DirName || strings.HasPrefix(rel, DirName+string(filepath.Separator)) {
			failed[f] = ErrOutsideRoot
			continue
		}
		if _, err := os.Lstat(f); errors.Is(err, os.ErrNotExist) {
			continue
		}
		dest := filepath.Join(batch, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			failed[f] = err
			continue
		}
		if err := os.Rename(f, dest); err != nil {
			failed[f] = err
		}
	}
	return failed
}

// Empty deletes every batch that has waited at least Hold and reports how many it deleted. A batch is
// aged by its name, the moment it was made, because a move keeps a file's own times. An entry Put did
// not make is left alone.
func (b Bin) Empty() (int, error) {
	dir := filepath.Join(b.root, DirName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read quarantine: %w", err)
	}
	cutoff := b.now().Add(-Hold)
	removed := 0
	var errs []error
	for _, e := range entries {
		at, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil || !e.IsDir() || time.Unix(at, 0).After(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}
