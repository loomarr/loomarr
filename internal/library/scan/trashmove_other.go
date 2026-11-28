//go:build !linux && !darwin

package scan

import (
	"errors"
	"os"
)

// renameNoReplace refuses: with no rename that is sure not to overwrite, the file stays put.
func renameNoReplace(src, dst string) error {
	return &os.LinkError{Op: "rename", Old: src, New: dst, Err: errors.ErrUnsupported}
}
