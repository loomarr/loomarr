package scan

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/unix"
)

// SMB2 change notifications (#2151 decision 9): an SMB share tells its clients about changes
// any of them makes, which inotify on a CIFS mount cannot. The Linux client exposes them as one
// ioctl that blocks until the server reports a change anywhere under the watched folder
// (sharewatch_linux.go).

// shareRetry is how long a share's watch waits after it failed before trying again.
const shareRetry = time.Minute

// shareChange is the folders under root one notification names; all of root when the server
// reported a change without naming it.
type shareChange struct {
	root string
	dirs []string
}

// watchShare reports root's changes until ctx ends. A server or kernel without change
// notifications stops it for good; any other failure is retried after shareRetry.
//
// The ioctl cannot be interrupted: after ctx ends, the call in flight returns with the share's
// next change and its result is dropped.
func watchShare(ctx context.Context, root string, retry time.Duration, out chan<- shareChange, failed func(error, bool)) {
	for ctx.Err() == nil {
		err := watchShareOnce(ctx, root, out)
		if ctx.Err() != nil {
			return
		}
		permanent := errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL)
		failed(err, permanent)
		if permanent {
			return
		}
		t := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// parseNotify reads FILE_NOTIFY_INFORMATION records: each a u32 offset to the next (0 for the
// last), a u32 action, a u32 name length in bytes and the name in UTF-16LE, "\" between folders.
func parseNotify(data []byte) []string {
	var names []string
	for off := 0; off+12 <= len(data); {
		next := int(binary.LittleEndian.Uint32(data[off:]))
		size := int(binary.LittleEndian.Uint32(data[off+8:]))
		start := off + 12
		if size%2 == 0 && start+size <= len(data) {
			units := make([]uint16, size/2)
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(data[start+2*i:])
			}
			names = append(names, strings.ReplaceAll(string(utf16.Decode(units)), `\`, "/"))
		}
		if next <= 0 {
			break
		}
		off += next
	}
	return names
}

// changedDirs is the folders to pass for names under root: each one's parent, which a pass walks
// whole, so a folder renamed or moved in is read with everything in it. No names is root itself.
func changedDirs(root string, names []string) []string {
	if len(names) == 0 {
		return []string{root}
	}
	seen := map[string]bool{}
	var dirs []string
	for _, name := range names {
		full := filepath.Join(root, filepath.FromSlash(name))
		if !under(full, []string{root}) || ignoredChange(full) {
			continue
		}
		dir := filepath.Dir(full)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
