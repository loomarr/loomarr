//go:build windows

package mediatools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// openConditioningRegularFile holds every component without following reparse points. Sharing
// read-only denies replacement while the next component and final bytes are opened.
func openConditioningRegularFile(path string) (*os.File, error) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if volume == "" || !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("conditioning media path is not absolute")
	}
	root := volume + string(os.PathSeparator)
	relative := strings.TrimPrefix(clean, root)
	components := []string{}
	if relative != "" {
		components = strings.Split(relative, string(os.PathSeparator))
	}
	paths := append([]string{root}, make([]string, len(components))...)
	current := root
	for index, component := range components {
		current = filepath.Join(current, component)
		paths[index+1] = current
	}

	handles := make([]windows.Handle, 0, len(paths))
	closeHandles := func() {
		for _, handle := range handles {
			_ = windows.CloseHandle(handle)
		}
	}
	for index, componentPath := range paths {
		final := index == len(paths)-1
		handle, err := openConditioningWindowsComponent(componentPath, final)
		if err != nil {
			closeHandles()
			return nil, err
		}
		handles = append(handles, handle)
	}
	final := handles[len(handles)-1]
	for _, handle := range handles[:len(handles)-1] {
		_ = windows.CloseHandle(handle)
	}
	return os.NewFile(uintptr(final), clean), nil
}

func openConditioningWindowsComponent(path string, final bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	access := uint32(windows.FILE_READ_ATTRIBUTES)
	if final {
		access = windows.GENERIC_READ
	}
	handle, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return windows.InvalidHandle, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, fmt.Errorf("reparse points are not regular media paths")
	}
	return handle, nil
}
