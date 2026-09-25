package media

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// HostMemAvailable reports the kernel's estimate of memory available for new work without
// swapping (Linux MemAvailable), in bytes. ok is false when the host does not expose it (non-Linux,
// a restricted /proc, or an unparseable file); callers must treat that as "unknown", never as zero.
func HostMemAvailable() (bytes int64, ok bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	return parseMemAvailable(f)
}

// parseMemAvailable extracts MemAvailable from /proc/meminfo content. The kernel reports kB.
func parseMemAvailable(r io.Reader) (int64, bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		name, rest, found := strings.Cut(scanner.Text(), ":")
		if !found || name != "MemAvailable" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, false
		}
		kib, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || kib < 0 {
			return 0, false
		}
		if len(fields) > 1 && fields[1] != "kB" {
			return 0, false
		}
		return kib * 1024, true
	}
	return 0, false
}
