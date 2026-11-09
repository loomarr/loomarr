package watermark

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// CacheIn returns the bug's file for key in dir, rendering it when absent. The caller holds the
// lock that serialises renders into dir.
func CacheIn(dir, key string, render func() (Bug, error)) (path string, w, h int, hit bool, err error) {
	sum := sha256.Sum256([]byte(key))
	path = filepath.Join(dir, hex.EncodeToString(sum[:10])+".png")
	if f, err := os.Open(path); err == nil {
		cfg, derr := png.DecodeConfig(f)
		_ = f.Close()
		if derr == nil {
			return path, cfg.Width, cfg.Height, true, nil
		}
	}
	bug, err := render()
	if err != nil {
		return "", 0, 0, false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, 0, false, err
	}
	if err := writePNGAtomic(path, bug.Straight); err != nil {
		return "", 0, 0, false, err
	}
	w, h = bug.Size()
	return path, w, h, false, nil
}

// PruneLeastRecentlyUsed removes the oldest-touched PNGs in dir beyond keep, never keepPath (the
// file just written).
func PruneLeastRecentlyUsed(dir string, keep int, keepPath string, log *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type used struct {
		path string
		at   time.Time
	}
	var files []used
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".png" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, used{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(files) <= keep {
		return
	}
	slices.SortFunc(files, func(a, b used) int { return a.at.Compare(b.at) })
	for _, f := range files[:len(files)-keep] {
		if f.path == keepPath {
			continue
		}
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			log.Warn("watermark: a stale preview could not be removed", "path", f.path, "err", err)
		}
	}
}

func writePNGAtomic(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bug-*")
	if err != nil {
		return err
	}
	if err := png.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
