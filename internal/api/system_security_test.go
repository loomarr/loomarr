package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
)

type fakeEncryption struct {
	status  api.EncryptionStatus
	rotated int
}

func (f *fakeEncryption) Status(context.Context) (api.EncryptionStatus, error) { return f.status, nil }
func (f *fakeEncryption) RotateDataKey(context.Context) error {
	f.rotated++
	return nil
}

func TestSystemEncryptionStatusAndRotationAreAdminOnly(t *testing.T) {
	fake := &fakeEncryption{status: api.EncryptionStatus{
		Enabled: true, InstallationKeyFingerprint: "sha256:1234", DataKeyCount: 2,
	}}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{Auth: testAuthorizer{}, Encryption: fake})
	srv := httptest.NewServer(h)
	defer srv.Close()
	for _, path := range []string{"/v1/system/security/encryption", "/v1/system/security/encryption/rotate"} {
		method := http.MethodGet
		if path[len(path)-6:] == "rotate" {
			method = http.MethodPost
		}
		if resp := do(t, srv, method, path, memberToken, ""); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("member %s %s = %d, want 403", method, path, resp.StatusCode)
		}
	}
	resp := do(t, srv, http.MethodGet, "/v1/system/security/encryption", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got api.EncryptionStatus
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got != fake.status {
		t.Fatalf("status = %#v, want %#v", got, fake.status)
	}
	resp = do(t, srv, http.MethodPost, "/v1/system/security/encryption/rotate", adminToken, "")
	if resp.StatusCode != http.StatusNoContent || fake.rotated != 1 {
		t.Fatalf("rotation = status %d calls %d", resp.StatusCode, fake.rotated)
	}
}
