//go:build !windows

package storagegovernor

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func platformMeasurement(path string) (Measurement, error) {
	existing, err := nearestExisting(path)
	if err != nil {
		return Measurement{}, err
	}
	filesystemID, err := platformFilesystemID(existing)
	if err != nil {
		return Measurement{}, err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(existing, &stat); err != nil {
		return Measurement{}, err
	}
	blockSize := int64(stat.Bsize)
	return Measurement{
		ID: filesystemID, TotalBytes: saturatingMultiply(int64(stat.Blocks), blockSize),
		FreeBytes: saturatingMultiply(int64(stat.Bavail), blockSize),
	}, nil
}

func platformFilesystemID(path string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return "", err
	}
	return fmt.Sprintf("unix:%d", uint64(stat.Dev)), nil
}
