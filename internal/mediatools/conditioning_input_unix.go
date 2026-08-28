//go:build !windows

package mediatools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openConditioningRegularFile binds validation and snapshot reads to one descriptor and refuses
// symlink traversal. The private snapshot then gives every media-tool invocation the same bytes.
func openConditioningRegularFile(path string) (*os.File, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("conditioning media path is not absolute")
	}
	// Hold each opened directory while resolving the next component. An ancestor therefore cannot
	// be exchanged for a symlink between validation and the final openat call.
	directoryFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	components := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	if len(components) == 1 && components[0] == "" {
		return os.NewFile(uintptr(directoryFD), clean), nil
	}
	for index, component := range components {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(components)-1 {
			flags |= unix.O_DIRECTORY
		} else {
			// O_NONBLOCK is inert for regular files and prevents a malicious FIFO from stalling before
			// the descriptor's type can be checked.
			flags |= unix.O_NONBLOCK
		}
		nextFD, openErr := unix.Openat(directoryFD, component, flags, 0)
		_ = unix.Close(directoryFD)
		if openErr != nil {
			return nil, openErr
		}
		directoryFD = nextFD
	}
	return os.NewFile(uintptr(directoryFD), clean), nil
}
