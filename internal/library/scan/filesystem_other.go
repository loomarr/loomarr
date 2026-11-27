//go:build !linux

package scan

import "golang.org/x/sys/unix"

// localFilesystem reports whether the kernel's file events (kqueue) see every change under path:
// the mount is local. On a network mount they miss changes made on another machine.
func localFilesystem(path string) bool {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return false
	}
	return fs.Flags&unix.MNT_LOCAL != 0
}

// cifsFilesystem reports whether path is on an SMB mount whose server reports its changes. The
// change-notification ioctl watchShare uses belongs to the Linux CIFS client, so no mount here
// qualifies; an SMB root is left to the hourly walk and the arrs' webhooks, as NFS is.
func cifsFilesystem(string) bool { return false }
