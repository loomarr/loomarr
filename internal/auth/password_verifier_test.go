package auth

import (
	"strings"
	"testing"
)

func TestPasswordVerifier_Argon2idPHCAndSalted(t *testing.T) {
	first, err := hashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if first == "same-password" || !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("hash is not the required Argon2id PHC encoding: %q", first)
	}
	if first == second {
		t.Fatal("same password produced the same hash; salts must be random")
	}
	if !verifyPassword(first, "same-password") || verifyPassword(first, "wrong-password") {
		t.Fatal("password verifier accepted/rejected the wrong value")
	}
}

func TestPasswordVerifier_MalformedAndOversizedFailClosed(t *testing.T) {
	for _, encoded := range []string{
		"",
		"not-phc",
		"$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$dGFn",
		"$argon2id$v=16$m=65536,t=3,p=4$c2FsdA$dGFn",
		"$argon2id$v=19$m=4294967295,t=4294967295,p=255$c2FsdA$dGFn",
		"$argon2id$v=19$m=65536,t=3,p=4$%%%$%%%",
	} {
		if verifyPassword(encoded, "password") {
			t.Errorf("malformed verifier accepted: %q", encoded)
		}
	}
}
