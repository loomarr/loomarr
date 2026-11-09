package asset

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

// CopyFile copies src to dst with no storage governor. See CopyFileWithStorage.
func CopyFile(src, dst string) error {
	return CopyFileWithStorage(context.Background(), src, dst, nil)
}

// CopyFileWithStorage copies a non-empty regular file to dst through a temporary name, reserving
// its size with governor when one is given and revalidating as it writes.
func CopyFileWithStorage(ctx context.Context, src, dst string, governor *storagegovernor.Governor) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		if err == nil {
			err = errors.New("source is not a non-empty regular file")
		}
		return err
	}
	var lease *storagegovernor.Lease
	if governor != nil {
		var decision storagegovernor.Decision
		lease, decision = governor.Reserve(ctx, storagegovernor.Request{
			Path: dst, Domain: storagegovernor.DomainFiller,
			EstimatedBytes: info.Size(), Mode: storagegovernor.Automatic,
		})
		if lease == nil {
			return storagegovernor.Pause("reserve cross-filesystem intake copy", decision)
		}
		defer lease.Release()
	}

	// ⚠ Written to a temp name and renamed into place, so a crash mid-copy cannot leave a
	// truncated file sitting at the hash's name — where the duplicate check would then treat it
	// as a complete clip and discard every future arrival of the real one.
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	buffer := make([]byte, 128<<10)
	var written int64
	for {
		n, readErr := in.Read(buffer)
		if n > 0 {
			if written > info.Size()-int64(n) {
				_ = out.Close()
				_ = os.Remove(tmp)
				return errors.New("source grew beyond its reserved intake size")
			}
			if _, err := out.Write(buffer[:n]); err != nil {
				_ = out.Close()
				_ = os.Remove(tmp)
				return err
			}
			written += int64(n)
			if lease != nil {
				decision := lease.Revalidate(ctx, written)
				if !decision.Allowed {
					_ = out.Close()
					_ = os.Remove(tmp)
					return storagegovernor.Pause("revalidate cross-filesystem intake copy", decision)
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = out.Close()
				_ = os.Remove(tmp)
				return readErr
			}
			break
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
