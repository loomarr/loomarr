//go:build windows

package storagegovernor

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const windowsPathBuffer = 32768

func platformMeasurement(path string) (Measurement, error) {
	existing, err := nearestExisting(path)
	if err != nil {
		return Measurement{}, err
	}
	volumePath, volumeID, err := windowsVolume(existing)
	if err != nil {
		return Measurement{}, err
	}
	pointer, err := windows.UTF16PtrFromString(volumePath)
	if err != nil {
		return Measurement{}, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free); err != nil {
		return Measurement{}, err
	}
	return Measurement{ID: volumeID, TotalBytes: clampUint64(total), FreeBytes: clampUint64(available)}, nil
}

func platformFilesystemID(path string) (string, error) {
	_, id, err := windowsVolume(path)
	return id, err
}

func windowsVolume(path string) (string, string, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", err
	}
	volumePathBuffer := make([]uint16, windowsPathBuffer)
	if err := windows.GetVolumePathName(pointer, &volumePathBuffer[0], uint32(len(volumePathBuffer))); err != nil {
		return "", "", err
	}
	volumePath := windows.UTF16ToString(volumePathBuffer)
	volumePathPointer, err := windows.UTF16PtrFromString(volumePath)
	if err != nil {
		return "", "", err
	}
	volumeNameBuffer := make([]uint16, windowsPathBuffer)
	if err := windows.GetVolumeNameForVolumeMountPoint(volumePathPointer, &volumeNameBuffer[0], uint32(len(volumeNameBuffer))); err != nil {
		return "", "", err
	}
	volumeName := windows.UTF16ToString(volumeNameBuffer)
	if volumeName == "" {
		return "", "", fmt.Errorf("volume identity is empty for %q", path)
	}
	return volumePath, "windows:" + volumeName, nil
}

func clampUint64(value uint64) int64 {
	const maximum = uint64(^uint64(0) >> 1)
	if value > maximum {
		return int64(maximum)
	}
	return int64(value)
}
