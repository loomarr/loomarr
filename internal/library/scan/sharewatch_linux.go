package scan

import (
	"context"
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/unix"
)

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
