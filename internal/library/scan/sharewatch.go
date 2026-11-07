package scan

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/unix"
)

// SMB2 change notifications (#2151 decision 9): an SMB share tells its clients about changes
// any of them makes, which inotify on a CIFS mount cannot. The Linux client exposes them as one
// ioctl that blocks until the server reports a change anywhere under the watched folder.

// cifsNotifyInfo is CIFS_IOC_NOTIFY_INFO: _IOWR(0xCF, 11, struct smb3_notify_info), whose packed
// head is a u32 completion filter, a bool watch_tree and a u32 data_len (9 bytes).
const cifsNotifyInfo = 3<<30 | notifyHead<<16 | 0xCF<<8 | 11

const notifyHead = 9

// notifyFilter is the changes that matter to the library: names (files and folders) appearing,
// going or being renamed, and a file's size or last write changing.
const notifyFilter = 0x1 | 0x2 | 0x8 | 0x10 // FILE_NAME | DIR_NAME | SIZE | LAST_WRITE

// notifyBuffer is how much of the server's answer one call keeps. A batch larger than that, like
// one the server could not list, arrives empty and is passed as the whole root.
const notifyBuffer = 64 << 10

// shareRetry is how long a share's watch waits after it failed before trying again.
const shareRetry = time.Minute

// cifsFilesystem reports whether path is on an SMB mount.
func cifsFilesystem(path string) bool {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return false
	}
	return int64(fs.Type) == unix.CIFS_SUPER_MAGIC || int64(fs.Type) == unix.SMB2_SUPER_MAGIC
}

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

func watchShareOnce(ctx context.Context, root string, out chan<- shareChange) error {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	buf := make([]byte, notifyHead+notifyBuffer)
	for ctx.Err() == nil {
		names, err := notifyOnce(fd, buf)
		if err != nil {
			return err
		}
		change := shareChange{root: root, dirs: changedDirs(root, names)}
		select {
		case out <- change:
		case <-ctx.Done():
		}
	}
	return nil
}

// notifyOnce waits for the server's next report under fd: the paths it names, relative to fd's
// folder, or none when it reported a change without naming it.
func notifyOnce(fd int, buf []byte) ([]string, error) {
	binary.LittleEndian.PutUint32(buf[0:4], notifyFilter)
	buf[4] = 1 // the whole tree
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(buf)-notifyHead))
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), cifsNotifyInfo, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return nil, errno
	}
	n := int(binary.LittleEndian.Uint32(buf[5:9]))
	return parseNotify(buf[notifyHead : notifyHead+min(n, len(buf)-notifyHead)]), nil
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
