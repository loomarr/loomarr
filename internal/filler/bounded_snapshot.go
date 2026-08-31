package filler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/mediatools"
)

// snapshotOwnedFile is used only where a source snapshot is an ownership boundary. It binds the
// copy to one open regular-file descriptor and never exposes a partial destination.
func snapshotOwnedFile(ctx context.Context, source, destination string) error {
	return snapshotOwnedFileBounded(ctx, source, destination, mediatools.ConditioningMaxSnapshotBytes, nil)
}

// snapshotOwnedFileBounded is the private deterministic seam for proving the ownership copy.
// Production callers use snapshotOwnedFile, which is permanently bound to the conditioning cap.
func snapshotOwnedFileBounded(ctx context.Context, source, destination string, limit int64, afterChunk func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return fmt.Errorf("snapshot source is not a bounded regular file")
	}
	length := info.Size()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".snapshot-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	remove := true
	defer func() {
		_ = tmp.Close()
		if remove {
			_ = os.Remove(tmpName)
		}
	}()
	buf := make([]byte, 128<<10)
	var copied int64
	for copied < length {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := length - copied
		if int64(len(buf)) > want {
			buf = buf[:want]
		}
		n, readErr := io.ReadFull(in, buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				return err
			}
			copied += int64(n)
			if afterChunk != nil {
				afterChunk()
			}
		}
		if readErr != nil {
			return fmt.Errorf("snapshot short read: %w", readErr)
		}
	}
	var extra [1]byte
	if n, readErr := in.Read(extra[:]); n != 0 || !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("snapshot source grew beyond declared length")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	finalInfo, err := in.Stat()
	if err != nil || finalInfo.Size() != length {
		return fmt.Errorf("snapshot source length changed during copy")
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	remove = false
	return nil
}
