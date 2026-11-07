// Package secretprotection seals reversibly stored database secrets into versioned,
// context-bound envelopes. Key provisioning and durable wrapped-key storage sit above
// this package; callers never handle cipher primitives directly.
package secretprotection

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const envelopePrefix = "loomarr-secret:v1:"

var defaultRandom io.Reader = rand.Reader

// ErrDataKeyUnavailable means the envelope names a DEK this process has not
// loaded. A replica may refresh its wrapped keyring and retry once.
var ErrDataKeyUnavailable = errors.New("secret protection: data key unavailable")

// IsEnvelope reports whether a value uses a recognized secret envelope. It is
// used only to distinguish legacy plaintext during forward migration.
func IsEnvelope(value string) bool { return strings.HasPrefix(value, envelopePrefix) }

// DataKey is one versioned data-encryption key. Material is never serialized by
// this package; the wrapping layer owns its durable representation.
type DataKey struct {
	ID       string
	Material [32]byte
}

// Record supplies authenticated context. An envelope sealed for one logical
// record or field cannot be opened as another.
type Record struct {
	Kind  string
	ID    string
	Field string
}

// Protector seals with the active key and opens with any retained readable key.
type Protector struct {
	activeID string
	keys     map[string]cipher.AEAD
	random   io.Reader
}

// New constructs a Protector. Retired keys are read-only; the active key always
// owns new envelopes.
func New(active DataKey, retired []DataKey, random io.Reader) (*Protector, error) {
	if active.ID == "" || strings.Contains(active.ID, ":") {
		return nil, errors.New("secret protection: invalid active data-key id")
	}
	if random == nil {
		random = rand.Reader
	}
	p := &Protector{activeID: active.ID, keys: make(map[string]cipher.AEAD, len(retired)+1), random: random}
	for _, key := range append([]DataKey{active}, retired...) {
		if key.ID == "" || strings.Contains(key.ID, ":") {
			return nil, errors.New("secret protection: invalid data-key id")
		}
		if _, exists := p.keys[key.ID]; exists {
			return nil, errors.New("secret protection: duplicate data-key id")
		}
		block, err := aes.NewCipher(key.Material[:])
		if err != nil {
			return nil, fmt.Errorf("secret protection: initialize data key: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("secret protection: initialize authenticated encryption: %w", err)
		}
		p.keys[key.ID] = aead
	}
	return p, nil
}

// Seal encrypts plaintext with a fresh nonce and binds it to record.
func (p *Protector) Seal(record Record, plaintext []byte) (string, error) {
	if p == nil {
		return "", errors.New("secret protection: unavailable")
	}
	if err := validateRecord(record); err != nil {
		return "", err
	}
	aead := p.keys[p.activeID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(p.random, nonce); err != nil {
		return "", fmt.Errorf("secret protection: generate nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, authenticatedContext(record))
	payload := append(nonce, sealed...)
	return envelopePrefix + p.activeID + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

// Open authenticates and decrypts one versioned envelope for record.
func (p *Protector) Open(record Record, envelope string) ([]byte, error) {
	if p == nil {
		return nil, errors.New("secret protection: unavailable")
	}
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(envelope, envelopePrefix) {
		return nil, errors.New("secret protection: unsupported envelope")
	}
	rest := strings.TrimPrefix(envelope, envelopePrefix)
	keyID, encoded, ok := strings.Cut(rest, ":")
	if !ok || keyID == "" || encoded == "" {
		return nil, errors.New("secret protection: malformed envelope")
	}
	aead, ok := p.keys[keyID]
	if !ok {
		return nil, ErrDataKeyUnavailable
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("secret protection: malformed envelope")
	}
	nonce, ciphertext := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, authenticatedContext(record))
	if err != nil {
		return nil, errors.New("secret protection: authentication failed")
	}
	return plaintext, nil
}

func authenticatedContext(record Record) []byte {
	return []byte(record.Kind + "\x00" + record.ID + "\x00" + record.Field)
}

func validateRecord(record Record) error {
	if record.Kind == "" || record.ID == "" || record.Field == "" {
		return errors.New("secret protection: incomplete record context")
	}
	if strings.ContainsRune(record.Kind, '\x00') || strings.ContainsRune(record.ID, '\x00') || strings.ContainsRune(record.Field, '\x00') {
		return errors.New("secret protection: invalid record context")
	}
	return nil
}
