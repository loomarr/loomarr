package fillerbakeoffio_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
)

type strictDocument struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func TestReadStrictJSONRejectsAmbiguousDocuments(t *testing.T) {
	t.Parallel()

	invalidUTF8 := append([]byte(`{"name":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","items":[]}`)...)
	tests := map[string][]byte{
		"unknown field":  []byte(`{"name":"alpha","items":[],"extra":true}`),
		"case variant":   []byte(`{"Name":"alpha","items":[]}`),
		"duplicate name": []byte(`{"name":"first","name":"second","items":[]}`),
		"invalid UTF-8":  invalidUTF8,
		"trailing value": []byte(`{"name":"alpha","items":[]}{}`),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "document.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := fillerbakeoffio.ReadStrictJSON[strictDocument](path); err == nil {
				t.Fatal("ReadStrictJSON accepted an ambiguous document")
			}
		})
	}
}

func TestReadStrictJSONPreservesNilAndEmptyCollections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		wantNil bool
	}{
		{name: "null", data: `{"name":"alpha","items":null}`, wantNil: true},
		{name: "empty", data: `{"name":"alpha","items":[]}`, wantNil: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "document.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := fillerbakeoffio.ReadStrictJSON[strictDocument](path)
			if err != nil {
				t.Fatal(err)
			}
			if (got.Items == nil) != test.wantNil || len(got.Items) != 0 {
				t.Fatalf("items = %#v, want nil=%t and length 0", got.Items, test.wantNil)
			}
		})
	}
}

func TestImmutableJSONCanonicalHashAndStrictRoundTrip(t *testing.T) {
	t.Parallel()

	want := strictDocument{Name: "alpha", Items: []string{}}
	path := filepath.Join(t.TempDir(), "document.json")
	if err := fillerbakeoffio.WriteImmutableJSON(path, ".strict-*", want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const wantHash = "5609d16ddd08b226203f229fdb378f558dc3a04bc4f8db4b7f5de5387d1afd07"
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != wantHash {
		t.Fatalf("canonical hash = %s, want %s\nbytes:\n%s", got, wantHash, data)
	}
	if !bytes.Equal(data, []byte("{\n  \"name\": \"alpha\",\n  \"items\": []\n}\n")) {
		t.Fatalf("canonical bytes changed:\n%s", data)
	}
	got, err := fillerbakeoffio.ReadStrictJSON[strictDocument](path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func BenchmarkReadStrictJSON(b *testing.B) {
	path := filepath.Join(b.TempDir(), "document.json")
	data := []byte(`{"name":"alpha","items":["one","two","three"]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := fillerbakeoffio.ReadStrictJSON[strictDocument](path); err != nil {
			b.Fatal(err)
		}
	}
}
