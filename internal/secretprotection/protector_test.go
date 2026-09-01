package secretprotection_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

func TestProtectorSealsAndOpensASecret(t *testing.T) {
	t.Parallel()

	key := secretprotection.DataKey{
		ID:       "dek-1",
		Material: [32]byte{0x42, 0x19, 0x7a, 0x2c, 0x91, 0xe3, 0x54, 0x08},
	}
	nonces := bytes.NewReader(bytes.Repeat([]byte{0xa5}, 32))
	protector, err := secretprotection.New(key, nil, nonces)
	if err != nil {
		t.Fatalf("new protector: %v", err)
	}

	record := secretprotection.Record{Kind: "setting", ID: "notifications.smtp.password", Field: "value"}
	envelope, err := protector.Seal(record, []byte("smtp-password"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if envelope == "smtp-password" || bytes.Contains([]byte(envelope), []byte("smtp-password")) {
		t.Fatalf("envelope contains plaintext: %q", envelope)
	}

	plaintext, err := protector.Open(record, envelope)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plaintext) != "smtp-password" {
		t.Fatalf("open returned %q, want original secret", plaintext)
	}
}

func TestProtectorRefusesAnUnboundSecret(t *testing.T) {
	t.Parallel()

	protector, err := secretprotection.New(secretprotection.DataKey{
		ID:       "dek-1",
		Material: [32]byte{0x61, 0x09, 0x88},
	}, nil, bytes.NewReader(bytes.Repeat([]byte{0x2d}, 32)))
	if err != nil {
		t.Fatalf("new protector: %v", err)
	}

	if _, err := protector.Seal(secretprotection.Record{}, []byte("must-not-be-unbound")); err == nil {
		t.Fatal("seal accepted a secret with no authenticated record context")
	}
}

func TestProtectorUsesFreshNoncesAndRejectsTamperingOrSubstitution(t *testing.T) {
	t.Parallel()
	key := secretprotection.DataKey{ID: "dek-context", Material: [32]byte{0x91, 0x28, 0x47}}
	nonces := append(bytes.Repeat([]byte{0x11}, 12), bytes.Repeat([]byte{0x22}, 12)...)
	protector, err := secretprotection.New(key, nil, bytes.NewReader(nonces))
	if err != nil {
		t.Fatal(err)
	}
	record := secretprotection.Record{Kind: "setting", ID: "library.token", Field: "value"}
	first, err := protector.Seal(record, []byte("same-plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := protector.Seal(record, []byte("same-plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two writes reused a nonce")
	}
	other := secretprotection.Record{Kind: "setting", ID: "seerr.api_key", Field: "value"}
	if _, err := protector.Open(other, first); err == nil {
		t.Fatal("ciphertext opened under a different record identity")
	}
	position := strings.LastIndex(first, ":") + 1
	last := first[position]
	replacement := byte('A')
	if last == replacement {
		replacement = 'B'
	}
	tampered := first[:position] + string(replacement) + first[position+1:]
	if _, err := protector.Open(record, tampered); err == nil {
		t.Fatal("tampered ciphertext authenticated")
	}
}
