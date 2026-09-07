package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private output is not a regular directory")
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func readInventory(path string) (fillercorpus.Inventory, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fillercorpus.Inventory{}, "", err
	}
	value, err := fillercorpus.DecodeInventoryBytes(data)
	if err != nil {
		return fillercorpus.Inventory{}, "", err
	}
	sum := sha256.Sum256(data)
	return value, hex.EncodeToString(sum[:]), nil
}
