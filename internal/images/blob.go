package images

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The on-disk half of the image service: content-addressed files under images.dir (§22).
//
// Layout, sharded two levels by hash prefix so no directory ever holds a hundred thousand entries
// (which is where ext4 lookups and, worse, `ls` in a support session both fall over):
//
//	<dir>/orig/ab/cd/abcdef0123….jpg
//	<dir>/drv/ab/cd/abcdef0123…_w320.webp
//
// ⚠ Originals and derivatives are separated at the TOP level, not mixed and distinguished by
// suffix, because their lifecycles differ completely: `drv/` is disposable cache the GC evicts
// under a budget, while `orig/` holds the only copy of an upload. A future "clear the image cache"
// operation must be able to delete a whole subtree without a filter that could be got wrong.

// blobStore is the filesystem. Kept as a struct rather than free functions so the root is bound
// once and no call site can be handed a path relative to the wrong directory.
type blobStore struct{ dir string }

func newBlobStore(dir string) *blobStore { return &blobStore{dir: dir} }

// HashBytes returns the sha256 identity of some bytes, hex-encoded.
//
// ⚠ The full 64-hex-character digest, deliberately un-truncated. A shortened hash saves nothing
// that matters (the path is never typed by a human) and buys a collision domain — and a collision
// here does not corrupt one record, it silently serves one operator's uploaded icon in place of
// another's, forever, because the content address IS the identity.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// validHash guards every path built from caller input.
//
// ⚠ This is the containment boundary for the serve route, and it is stricter than a traversal
// check on purpose. Rather than sanitizing `..` out of an arbitrary string, a hash is required to
// be exactly 64 lowercase hex characters — a shape in which no separator, dot, or NUL can occur at
// all. A rejected input cannot escape the directory because it cannot contain anything that would
// let it. `filepath.Rel`-style containment is the fallback for paths we did not construct; here we
// construct all of them, so the stronger rule applies.
func validHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for i := range len(hash) {
		c := hash[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ErrBadHash is returned when a caller supplies something that is not a content hash. Distinct
// from ErrNotFound so the API layer can tell a malformed URL from a missing image — but ⚠ both
// must surface to the client as 404, since a distinct status would confirm which hashes exist.
var ErrBadHash = errors.New("images: malformed hash")

// shard splits a hash into its two directory levels.
func shard(hash string) (string, string) { return hash[0:2], hash[2:4] }

// OriginalPath is where an image's source bytes live. `ext` carries the leading dot.
func (b *blobStore) OriginalPath(hash, ext string) (string, error) {
	if !validHash(hash) {
		return "", ErrBadHash
	}
	a, c := shard(hash)
	return filepath.Join(b.dir, "orig", a, c, hash+ext), nil
}

// DerivativePath is where one rendition lives. The width and format are in the FILENAME rather
// than in nested directories so a single stat answers "does this rendition exist", and so the
// whole set for one image is adjacent on disk and in a directory listing.
func (b *blobStore) DerivativePath(hash string, width int, f Format) (string, error) {
	if !validHash(hash) {
		return "", ErrBadHash
	}
	a, c := shard(hash)
	name := fmt.Sprintf("%s_w%d.%s", hash, width, f.Ext())
	return filepath.Join(b.dir, "drv", a, c, name), nil
}

// Write stores bytes at path atomically.
//
// ⚠ **Write-temp-then-rename, not write-in-place, and the reason is specific to this service.**
// A half-written derivative is indistinguishable from a complete one by the only test the read
// path can cheaply apply — the file exists and is non-empty — so a crash or a full disk mid-write
// would leave a truncated image that is then served, cached with `immutable`, and never
// regenerated. Rename is atomic within a filesystem, so a reader sees either nothing or the whole
// file. §10's artwork pass learned the same lesson the harder way, by adopting empty ffmpeg
// output as "already generated".
func (b *blobStore) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("images: mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("images: temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Any failure from here on must not leave the scratch file behind; the deferred remove is a
	// no-op once the rename has succeeded.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("images: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("images: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("images: chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("images: rename into place %s: %w", path, err)
	}
	return nil
}

// WriteFrom streams a reader to path, capped at maxBytes.
//
// ⚠ The cap is enforced on the READ, never on a declared length. A multipart part's Size and a
// remote response's Content-Length are both claims by the other side; the only number that bounds
// what actually lands on our disk is how many bytes we agree to copy.
func (b *blobStore) WriteFrom(path string, r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := readCapped(r, maxBytes)
	if err != nil {
		return nil, err
	}
	if err := b.Write(path, data); err != nil {
		return nil, err
	}
	return data, nil
}

// readCapped reads at most maxBytes, treating one byte more as a refusal rather than a truncation.
//
// ⚠ The `+1` is the whole mechanism: LimitReader alone would silently hand back a truncated image,
// which decodes as a corrupt file rather than as an over-large one. Reading one byte past the
// limit is what makes "too big" distinguishable from "exactly at the limit".
//
// Shared by the upload path and the remote fetch because the rule is the same in both: the cap is
// enforced on what we actually read, never on a Content-Length or a multipart Size, both of which
// are claims by the other side.
func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("images: read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("images: larger than the %d byte limit", maxBytes)
	}
	return data, nil
}

// Stat reports a file's size, or ok=false when it is absent OR EMPTY.
//
// ⚠ "Exists" is the wrong question and this codebase has paid for asking it. A zero-length file is
// what a killed encoder leaves behind, and treating it as present means recording a derivative
// that renders as a BROKEN image rather than an absent one — visibly worse than having none, since
// the absent case has a designed fallback and the broken case does not.
func (b *blobStore) Stat(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return 0, false
	}
	return info.Size(), true
}

// Remove deletes a file, treating "already gone" as success — the GC and the failure-cleanup paths
// both race with each other and with a restart, and neither should log an error for reaching the
// state it wanted.
func (b *blobStore) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("images: remove %s: %w", path, err)
	}
	return nil
}

// RemoveAllFor deletes every derivative of one image, leaving the original alone. Used when a
// rendition set is invalidated (a format list change) and by the GC's budget eviction.
func (b *blobStore) RemoveAllFor(hash string) error {
	if !validHash(hash) {
		return ErrBadHash
	}
	a, c := shard(hash)
	dir := filepath.Join(b.dir, "drv", a, c)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("images: read %s: %w", dir, err)
	}
	// The shard directory is shared with other images, so this filters by name rather than
	// removing the directory — deleting the shard would take unrelated images with it.
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), hash+"_w") {
			if err := b.Remove(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// extForMIME maps an accepted input type to the extension its original is stored under. Only the
// allowlisted inputs appear; anything else never reaches storage.
func extForMIME(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ".bin"
}
