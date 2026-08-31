package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	argonMemory  uint32 = 64 * 1024
	argonTime    uint32 = 3
	argonThreads uint8  = 4
	argonSaltLen        = 16
	argonKeyLen  uint32 = 32
)

// hashPassword is the single password-storage seam. The PHC encoding carries
// enough versioning metadata to reject an unsupported format without guessing.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	tag := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(tag)), nil
}

// verifyPassword fails closed on every parse error. Parameters are parsed and
// checked against the one supported profile before Argon2 allocates memory, so a
// corrupt database value cannot turn login into an unbounded resource request.
func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false
	}
	memory, ok := parsePHCParam(params[0], "m=", 32)
	if !ok || uint32(memory) != argonMemory {
		return false
	}
	timeCost, ok := parsePHCParam(params[1], "t=", 32)
	if !ok || uint32(timeCost) != argonTime {
		return false
	}
	threads, ok := parsePHCParam(params[2], "p=", 8)
	if !ok || uint8(threads) != argonThreads {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != argonSaltLen {
		return false
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) != int(argonKeyLen) {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memory), uint8(threads), argonKeyLen)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// verifyStoredLocalPassword also recognizes the one legacy format Loomarr used
// before Argon2id. The bool result requests an immediate rewrite after a valid
// login; callers must never preserve bcrypt beyond that successful verification.
func verifyStoredLocalPassword(encoded, password string) (valid, needsUpgrade bool) {
	if strings.HasPrefix(encoded, "$argon2id$") {
		return verifyPassword(encoded, password), false
	}
	if strings.HasPrefix(encoded, "$2a$") || strings.HasPrefix(encoded, "$2b$") || strings.HasPrefix(encoded, "$2y$") {
		if bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil {
			return true, true
		}
	}
	return false, false
}

func parsePHCParam(value, prefix string, bits int) (uint64, bool) {
	if !strings.HasPrefix(value, prefix) {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, bits)
	return n, err == nil
}
