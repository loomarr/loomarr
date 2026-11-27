//go:build !linux

package scan

import (
	"context"

	"golang.org/x/sys/unix"
)

// localFilesystem reports whether the watcher follows path through the kernel's file events. Off
// Linux no root qualifies: fsnotify watches through kqueue there, which holds an open descriptor
// for every folder and file it watches, so a library's worth of them would exhaust the process's
// descriptors and starve the store and the HTTP server. Every root is left to the hourly walk and
// the arrs' webhooks, as a network mount is on Linux.
func localFilesystem(string) bool { return false }

// cifsFilesystem reports whether path is on an SMB mount whose server reports its changes. The
// change-notification ioctl watchShare uses belongs to the Linux CIFS client, so no mount
// qualifies here.
func cifsFilesystem(string) bool { return false }

// watchShareOnce has no change notifications to wait on here, which stops a share's watch for good.
func watchShareOnce(context.Context, string, chan<- shareChange) error { return unix.ENOTTY }
