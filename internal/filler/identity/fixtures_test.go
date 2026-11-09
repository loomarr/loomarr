package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
)

func preserveSourceMaster(ctx context.Context, clipDir, sourcePath, clipHash string, tags asset.SidecarTags) (asset.Identity, error) {
	return asset.PreserveSourceMasterWithStorage(ctx, clipDir, sourcePath, clipHash, tags, nil)
}

// writeContentAddressedClip files body under dir at the path its content hash names, as intake
// would, and returns the hash.
func writeContentAddressedClip(t *testing.T, dir string, body []byte, ext string) string {
	t.Helper()
	tmp := filepath.Join(dir, "source"+ext)
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := asset.ClipID(tmp)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, cliplibrary.ClipRelPath(hash, ext))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		t.Fatal(err)
	}
	return hash
}
