package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeBackups is a scriptable BackupsService. It records the name Open was asked for, so
// the traversal assertions can prove a rejected request never reached the service at all
// versus being rejected inside it.
type fakeBackups struct {
	list     api.BackupList
	content  api.BackupContent
	openErr  error
	openedAs []string
	ran      bool
}

func (f *fakeBackups) List(context.Context) (api.BackupList, error) { return f.list, nil }

func (f *fakeBackups) Open(_ context.Context, name string) (api.BackupContent, error) {
	f.openedAs = append(f.openedAs, name)
	if f.openErr != nil {
		return api.BackupContent{}, f.openErr
	}
	return f.content, nil
}

func (f *fakeBackups) Run(context.Context) (api.BackupEntry, error) {
	f.ran = true
	return api.BackupEntry{Name: "loomarr-2026-07-29-033000.db", Bytes: 4096, WrittenAt: 1_700_000_000}, nil
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func serverWithBackups(t *testing.T, svc api.BackupsService) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:   st,
		Auth:    testAuthorizer{},
		Log:     slog.New(slog.DiscardHandler),
		Backups: svc,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// §19 negative: a backup is the whole instance including every generated secret, so
// every route here is admin-only — including the download, which is a plain mux handler
// and therefore does NOT inherit Huma's requireAdmin.
func TestSystemBackups_RequiresAdmin(t *testing.T) {
	fake := &fakeBackups{content: api.BackupContent{Name: "loomarr-2026-07-29-033000.db"}}
	srv := serverWithBackups(t, fake)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/system/backups"},
		{http.MethodPost, "/v1/system/backups"},
		{http.MethodGet, "/v1/system/backups/loomarr-2026-07-29-033000.db"},
	} {
		resp := do(t, srv, tc.method, tc.path, "", "") // no token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
	if fake.ran {
		t.Error("a request with no admin token wrote a backup")
	}
	if len(fake.openedAs) != 0 {
		t.Errorf("a request with no admin token reached Open: %v", fake.openedAs)
	}
}

// A Postgres install wires no service. The routes report 501 rather than an empty list,
// because "no backups" and "in-app backup is not offered on this backend" are different
// facts and only one of them means something is wrong.
func TestSystemBackups_NotConfigured501(t *testing.T) {
	srv := serverWithBackups(t, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/system/backups"},
		{http.MethodPost, "/v1/system/backups"},
		{http.MethodGet, "/v1/system/backups/loomarr-2026-07-29-033000.db"},
	} {
		resp := do(t, srv, tc.method, tc.path, adminToken, "")
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s %s with no service → %d, want 501", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// The list carries the policy alongside the files: the page has to say "nightly at 03:30,
// keeps 7" and neither number is derivable from the file list.
func TestSystemBackups_ListReportsFilesAndPolicy(t *testing.T) {
	srv := serverWithBackups(t, &fakeBackups{list: api.BackupList{
		Supported: true, Dir: "/data/backups", Schedule: "0 30 3 * * *", Retain: 7,
		Backups: []api.BackupEntry{
			{Name: "loomarr-2026-07-29-033000.db", Bytes: 4096, WrittenAt: 1_700_000_000},
		},
	}})
	resp := do(t, srv, http.MethodGet, "/v1/system/backups", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d, want 200", resp.StatusCode)
	}
	var got api.BackupList
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Retain != 7 || got.Schedule != "0 30 3 * * *" || got.Dir != "/data/backups" {
		t.Errorf("policy = retain %d, schedule %q, dir %q", got.Retain, got.Schedule, got.Dir)
	}
	if len(got.Backups) != 1 || got.Backups[0].Bytes != 4096 {
		t.Errorf("backups = %+v", got.Backups)
	}
}

// The handler serves the bytes the service resolved, with a filename the browser will
// save under. The traversal gate itself is asserted against the REAL service in
// internal/app (TestBackupsService_OpenRejectsTraversal) — a fake here would only prove
// the fake validates.
func TestSystemBackups_DownloadServesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "loomarr-2026-07-29-033000.db", "snapshot-bytes")
	srv := serverWithBackups(t, &fakeBackups{content: api.BackupContent{
		Name: "loomarr-2026-07-29-033000.db", Path: path, Bytes: 14,
	}})

	resp := do(t, srv, http.MethodGet, "/v1/system/backups/loomarr-2026-07-29-033000.db", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download → %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "snapshot-bytes" {
		t.Errorf("downloaded %q, want the snapshot bytes", body)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="loomarr-2026-07-29-033000.db"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

// A pruned backup is gone, and the endpoint says so plainly rather than 500ing.
func TestSystemBackups_DownloadMissingIs404(t *testing.T) {
	srv := serverWithBackups(t, &fakeBackups{openErr: errors.New("not found")})
	resp := do(t, srv, http.MethodGet, "/v1/system/backups/loomarr-2026-07-29-033000.db", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("download of a pruned backup → %d, want 404", resp.StatusCode)
	}
}

// "Back up now" writes one and reports what it wrote, so the page can show the new row
// without a second round trip.
func TestSystemBackups_RunWritesAndReports(t *testing.T) {
	fake := &fakeBackups{}
	srv := serverWithBackups(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/backups", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run → %d, want 200", resp.StatusCode)
	}
	if !fake.ran {
		t.Error("run did not reach the service")
	}
	var got api.BackupEntry
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "loomarr-2026-07-29-033000.db" || got.Bytes != 4096 {
		t.Errorf("entry = %+v", got)
	}
}
