package secretprotection

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const wrappedKeyPrefix = "loomarr-dek:v1:"

// InstallationKey is the key-encryption key kept outside the database.
type InstallationKey [32]byte

// Fingerprint returns a short, non-secret identity suitable for diagnostics
// and database mismatch detection. It cannot be used to recover the key.
func (k InstallationKey) Fingerprint() string {
	sum := sha256.Sum256(k[:])
	return "sha256:" + fmt.Sprintf("%x", sum[:12])
}

// WrappedDataKey is the database-safe representation of one DEK.
type WrappedDataKey struct {
	ID        string
	Wrapped   string
	Active    bool
	CreatedAt time.Time
}

// KeyWrapper wraps data-encryption keys for durable database storage.
type KeyWrapper struct {
	aead   cipher.AEAD
	random io.Reader
}

// NewKeyWrapper constructs a wrapper from the installation key.
func NewKeyWrapper(key InstallationKey, random io.Reader) (*KeyWrapper, error) {
	if random == nil {
		random = rand.Reader
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("secret protection: initialize installation key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret protection: initialize key wrapping: %w", err)
	}
	return &KeyWrapper{aead: aead, random: random}, nil
}

// Wrap encrypts one data key and authenticates its stable identity.
func (w *KeyWrapper) Wrap(key DataKey) (string, error) {
	if w == nil {
		return "", errors.New("secret protection: key wrapper unavailable")
	}
	if key.ID == "" || strings.ContainsRune(key.ID, '\x00') {
		return "", errors.New("secret protection: invalid data-key id")
	}
	nonce := make([]byte, w.aead.NonceSize())
	if _, err := io.ReadFull(w.random, nonce); err != nil {
		return "", fmt.Errorf("secret protection: generate wrapping nonce: %w", err)
	}
	sealed := w.aead.Seal(nil, nonce, key.Material[:], wrappedKeyContext(key.ID))
	payload := append(nonce, sealed...)
	return wrappedKeyPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// Unwrap authenticates and recovers one data key.
func (w *KeyWrapper) Unwrap(id, wrapped string) (DataKey, error) {
	if w == nil {
		return DataKey{}, errors.New("secret protection: key wrapper unavailable")
	}
	if id == "" || strings.ContainsRune(id, '\x00') || !strings.HasPrefix(wrapped, wrappedKeyPrefix) {
		return DataKey{}, errors.New("secret protection: malformed wrapped data key")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(wrapped, wrappedKeyPrefix))
	if err != nil || len(payload) < w.aead.NonceSize()+w.aead.Overhead() {
		return DataKey{}, errors.New("secret protection: malformed wrapped data key")
	}
	nonce, ciphertext := payload[:w.aead.NonceSize()], payload[w.aead.NonceSize():]
	material, err := w.aead.Open(nil, nonce, ciphertext, wrappedKeyContext(id))
	if err != nil || len(material) != 32 {
		return DataKey{}, errors.New("secret protection: wrapped data-key authentication failed")
	}
	key := DataKey{ID: id}
	copy(key.Material[:], material)
	return key, nil
}

func wrappedKeyContext(id string) []byte {
	return []byte("loomarr-data-key\x00" + id)
}
