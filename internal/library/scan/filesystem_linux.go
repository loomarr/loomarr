package scan

import "golang.org/x/sys/unix"

// Network and userspace filesystems, on which inotify misses changes made elsewhere.
var remoteFilesystems = map[int64]bool{
	unix.CIFS_SUPER_MAGIC: true,
	unix.SMB2_SUPER_MAGIC: true,
	unix.SMB_SUPER_MAGIC:  true,
	unix.NFS_SUPER_MAGIC:  true,
	unix.FUSE_SUPER_MAGIC: true,
	unix.V9FS_MAGIC:       true,
	unix.AFS_SUPER_MAGIC:  true,
	unix.CEPH_SUPER_MAGIC: true,
}

// localFilesystem reports whether inotify sees every change under path.
func localFilesystem(path string) bool {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return false
	}
	return !remoteFilesystems[int64(fs.Type)]
}

// cifsFilesystem reports whether path is on an SMB mount.
func cifsFilesystem(path string) bool {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return false
	}
	return int64(fs.Type) == unix.CIFS_SUPER_MAGIC || int64(fs.Type) == unix.SMB2_SUPER_MAGIC
}
