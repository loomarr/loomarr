package scan

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// readableTimeout bounds one readability check. A NAS that has gone away can hang a directory
// open far longer; past this the folder counts as unreadable and the check moves on.
const readableTimeout = 5 * time.Second

// systemMountPrefixes hold mounts that never carry media.
var systemMountPrefixes = []string{"/proc", "/sys", "/dev", "/run", "/etc", "/boot", "/snap", "/var/lib/docker"}

// OSDisk is the real filesystem.
type OSDisk struct {
	// MountInfo is the mount table to read; "" means /proc/self/mountinfo.
	MountInfo string
}

// Readable opens the directory and lists one entry, within readableTimeout.
func (d OSDisk) Readable(ctx context.Context, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, readableTimeout)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- listable(path) }()
	select {
	case ok := <-done:
		return ok
	case <-ctx.Done():
		return false
	}
}

func listable(path string) bool {
	f, err := os.Open(path) //nolint:gosec // the path is a folder the maintainer or an authority named
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = f.ReadDir(1)
	return err == nil || errors.Is(err, io.EOF)
}

// Mounts lists mount points from the mount table, leaving out the root and system mounts.
func (d OSDisk) Mounts() []string {
	path := d.MountInfo
	if path == "" {
		path = "/proc/self/mountinfo"
	}
	f, err := os.Open(path) //nolint:gosec // a fixed system file, or a test fixture
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	return parseMountInfo(f)
}

// parseMountInfo reads the fifth field (the mount point) of each mountinfo line.
func parseMountInfo(r io.Reader) []string {
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		mount := unescapeMount(fields[4])
		if mount == "/" || seen[mount] || isSystemMount(mount) {
			continue
		}
		seen[mount] = true
		out = append(out, mount)
	}
	return out
}

func isSystemMount(mount string) bool {
	for _, p := range systemMountPrefixes {
		if mount == p || strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	return false
}

// unescapeMount undoes mountinfo's octal escapes for space, tab, newline and backslash.
func unescapeMount(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}
