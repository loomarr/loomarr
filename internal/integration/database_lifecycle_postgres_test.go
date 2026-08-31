//go:build integration

package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

const lifecycleImageVersion = "0.0.0-lifecycle"

// TestDatabaseLifecycleFreshPostgresStartsFromComposeProfile is the deployment
// boundary for a new PostgreSQL install. It deliberately does not inject
// DATABASE_URL: selecting the shipped postgres profile must be sufficient to boot
// Loomarr on that profile's database rather than silently retaining SQLite.
func TestDatabaseLifecycleFreshPostgresStartsFromComposeProfile(t *testing.T) {
	root := repositoryRoot(t)
	deployment := startLifecycleDeployment(t,
		[]string{"postgres"},
		[]string{filepath.Join(root, "docker", "compose.postgres.yaml")},
	)
	baseURL, compose := deployment.baseURL, deployment.compose
	assertLifecycleBackend(t, baseURL, "postgres")

	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":7001}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	restartCtx, cancelRestart := context.WithTimeout(context.Background(), time.Minute)
	defer cancelRestart()
	if out, err := compose(restartCtx, "restart", "loomarr"); err != nil {
		t.Fatalf("restart fresh Postgres deployment: %v\n%s", err, out)
	}
	if err := awaitReady(baseURL, 45*time.Second); err != nil {
		t.Fatalf("fresh Postgres deployment did not recover after restart: %v", err)
	}
	assertLifecycleBackend(t, baseURL, "postgres")
	assertLifecycleTitle(t, baseURL, 7001)
}

// TestDatabaseLifecycleMigratesSQLiteAndRestartsOnPostgres drives the supported
// operator workflow through HTTP. The source starts from the shipped SQLite
// deployment while the target Postgres service shares only the private Compose
// network; Migrate must drain the live generation, copy and verify the source,
// persist the target, and bring the same container back on Postgres.
func TestDatabaseLifecycleMigratesSQLiteAndRestartsOnPostgres(t *testing.T) {
	deployment := startLifecycleDeployment(t,
		[]string{"sqlite", "postgres"},
		nil,
	)
	baseURL, compose := deployment.baseURL, deployment.compose
	assertLifecycleBackend(t, baseURL, "sqlite")

	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/setup/bootstrap",
		[]byte(`{"username":"lifecycle-owner","password":"correct horse battery staple"}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/auth/login",
		[]byte(`{"username":"lifecycle-owner","password":"correct horse battery staple"}`))
	if response.StatusCode != http.StatusOK {
		decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	}
	cookies := response.Cookies()
	_ = response.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("local admin login did not issue a session cookie")
	}
	adminSession := cookies[0]

	// Seed through the same admin API an operator uses. This is intentionally not a
	// direct INSERT: the lifecycle test must prove an application-owned record survives.
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":4242}`))
	if response.StatusCode != http.StatusOK {
		defer func() { _ = response.Body.Close() }()
		t.Fatalf("seed source title = %d, want 200", response.StatusCode)
	}
	_ = response.Body.Close()

	target := "postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable"
	body, _ := json.Marshal(map[string]string{"dsn": target})
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/preflight", body)
	var preflight struct {
		Passed bool `json:"passed"`
	}
	decodeLifecycleResponse(t, response, http.StatusOK, &preflight)
	if !preflight.Passed {
		t.Fatal("Postgres target did not pass preflight")
	}

	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/backup", nil)
	var backup struct {
		Bytes int64 `json:"bytes"`
	}
	decodeLifecycleResponse(t, response, http.StatusOK, &backup)
	if backup.Bytes == 0 {
		t.Fatal("pre-migration backup is empty")
	}

	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/migrate", body)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})

	if err := awaitDatabaseBackend(baseURL, "postgres", 90*time.Second); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		logs, _ := compose(ctx, "logs", "--no-color", "loomarr", "postgres")
		t.Fatalf("migration did not restart on Postgres: %v\n%s", err, logs)
	}

	response = lifecycleCookieRequest(t, http.MethodGet, baseURL+"/v1/auth/me", nil, adminSession)
	var me struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	decodeLifecycleResponse(t, response, http.StatusOK, &me)
	if me.Name != "lifecycle-owner" || me.Role != "admin" {
		t.Fatalf("migrated session resolved to %+v, want lifecycle-owner admin", me)
	}
	assertLifecycleTitle(t, baseURL, 4242)

	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":4343}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})

	restartCtx, cancelRestart := context.WithTimeout(context.Background(), time.Minute)
	defer cancelRestart()
	if out, err := compose(restartCtx, "restart", "loomarr"); err != nil {
		t.Fatalf("restart migrated Loomarr container: %v\n%s", err, out)
	}
	if err := awaitReady(baseURL, 45*time.Second); err != nil {
		t.Fatalf("migrated Loomarr did not recover after container restart: %v", err)
	}
	assertLifecycleBackend(t, baseURL, "postgres")
	assertLifecycleTitle(t, baseURL, 4343)

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
	if out, err := compose(stopCtx, "stop", "loomarr"); err != nil {
		cancelStop()
		t.Fatalf("stop Postgres generation for rollback: %v\n%s", err, out)
	}
	cancelStop()
	rollback := `{"values":{"DATABASE_URL":"sqlite:///data/loomarr.db"}}`
	rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), 30*time.Second)
	out, err := compose(rollbackCtx,
		"run", "--rm", "--no-deps", "--entrypoint", "sh", "loomarr-init",
		"-ceu", `printf '%s\n' "$1" > /data/bootstrap.json; chmod 600 /data/bootstrap.json`,
		"sh", rollback)
	cancelRollback()
	if err != nil {
		t.Fatalf("restore SQLite bootstrap choice: %v\n%s", err, out)
	}
	startRollbackCtx, cancelRollbackStart := context.WithTimeout(context.Background(), 30*time.Second)
	if out, err := compose(startRollbackCtx, "start", "loomarr"); err != nil {
		cancelRollbackStart()
		t.Fatalf("start rolled-back SQLite generation: %v\n%s", err, out)
	}
	cancelRollbackStart()
	if err := awaitDatabaseBackend(baseURL, "sqlite", 60*time.Second); err != nil {
		t.Fatalf("rollback did not restart on SQLite: %v", err)
	}
	assertLifecycleTitle(t, baseURL, 4242)
	assertLifecycleTitleMissing(t, baseURL, 4343)
	response = lifecycleCookieRequest(t, http.MethodGet, baseURL+"/v1/auth/me", nil, adminSession)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
}

// TestDatabaseLifecycleInterruptedMigrationRecoversSQLiteAndRetries proves the
// process boundary rather than an injected Go callback. A large, valid SQLite
// fixture keeps the copy active long enough for the test to kill the real Loomarr
// container after the target schema exists. The target is disposable; the source
// must remain readable, writable, and eligible for a clean retry.
func TestDatabaseLifecycleInterruptedMigrationRecoversSQLiteAndRetries(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "loomarr.db")
	sourceVersion := seedLargeSQLiteFixture(t, databasePath, 100_000)
	if err := os.Chmod(dataDir, 0o777); err != nil {
		t.Fatalf("make fixture directory container-writable: %v", err)
	}
	if err := os.Chmod(databasePath, 0o666); err != nil {
		t.Fatalf("make fixture database container-writable: %v", err)
	}
	override := filepath.Join(t.TempDir(), "compose.lifecycle.yaml")
	overrideBody := fmt.Sprintf(`services:
  loomarr-init:
    volumes: !override
      - %q
  loomarr:
    volumes: !override
      - %q
`, dataDir+":/data", dataDir+":/data")
	if err := os.WriteFile(override, []byte(overrideBody), 0o600); err != nil {
		t.Fatalf("write lifecycle Compose override: %v", err)
	}

	deployment := startLifecycleDeployment(t,
		[]string{"sqlite", "postgres"},
		[]string{override},
	)
	baseURL, compose := deployment.baseURL, deployment.compose
	assertLifecycleBackend(t, baseURL, "sqlite")
	assertLifecycleTitle(t, baseURL, 99_999)

	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/setup/bootstrap",
		[]byte(`{"username":"interrupt-owner","password":"correct horse battery staple"}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/auth/login",
		[]byte(`{"username":"interrupt-owner","password":"correct horse battery staple"}`))
	if response.StatusCode != http.StatusOK {
		decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	}
	_ = response.Body.Close()

	target := "postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable"
	targetBody, _ := json.Marshal(map[string]string{"dsn": target})
	runMigrationGates(t, baseURL, targetBody)
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/migrate", targetBody)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})

	if err := awaitPostgresTable(compose, "titles", 30*time.Second); err != nil {
		t.Fatalf("migration never created its target schema: %v", err)
	}
	killCtx, cancelKill := context.WithTimeout(context.Background(), 15*time.Second)
	if out, err := compose(killCtx, "kill", "--signal", "KILL", "loomarr"); err != nil {
		cancelKill()
		t.Fatalf("interrupt active migration: %v\n%s", err, out)
	}
	cancelKill()
	startSourceCtx, cancelSource := context.WithTimeout(context.Background(), 30*time.Second)
	if out, err := compose(startSourceCtx, "start", "loomarr"); err != nil {
		cancelSource()
		t.Fatalf("restart interrupted source: %v\n%s", err, out)
	}
	cancelSource()
	if err := awaitDatabaseBackend(baseURL, "sqlite", 60*time.Second); err != nil {
		t.Fatalf("interrupted migration did not recover SQLite: %v", err)
	}
	assertLifecycleTitle(t, baseURL, 99_999)
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":200001}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})

	resetCtx, cancelReset := context.WithTimeout(context.Background(), 30*time.Second)
	out, err := compose(resetCtx, "exec", "--no-TTY", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "-U", "loomarr", "-d", "postgres",
		"-c", "DROP DATABASE loomarr WITH (FORCE)",
		"-c", "CREATE DATABASE loomarr OWNER loomarr")
	cancelReset()
	if err != nil {
		t.Fatalf("clear interrupted migration target: %v\n%s", err, out)
	}

	runMigrationGates(t, baseURL, targetBody)
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/migrate", targetBody)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	if err := awaitDatabaseBackend(baseURL, "postgres", 2*time.Minute); err != nil {
		t.Fatalf("clean retry did not reach Postgres: %v", err)
	}
	assertLifecycleTitle(t, baseURL, 99_999)
	assertLifecycleTitle(t, baseURL, 200_001)

	if got := postgresScalar(t, compose,
		"SELECT COUNT(*) FROM titles"); got != "100001" {
		t.Fatalf("migrated title count = %s, want 100001", got)
	}
	if got := postgresScalar(t, compose,
		"SELECT MAX(version_id) FROM goose_db_version WHERE is_applied"); got != sourceVersion {
		t.Fatalf("Postgres schema version = %s, want SQLite version %s", got, sourceVersion)
	}
	if got := postgresScalar(t, compose,
		"SELECT COUNT(*) FROM sessions s JOIN users u ON u.id = s.user_id"); got != "1" {
		t.Fatalf("migrated session→user relationships = %s, want 1", got)
	}
	if got := postgresScalar(t, compose,
		"SELECT COUNT(*) FROM sessions s LEFT JOIN users u ON u.id = s.user_id WHERE u.id IS NULL"); got != "0" {
		t.Fatalf("orphaned migrated sessions = %s, want 0", got)
	}
}

// TestDatabaseLifecycleFailedTargetRecheckStaysOnSQLite proves that admission
// is not authorization forever. The target passes the operator preflight, then
// changes before the offline copy. The process-owned recheck must fail, restart
// on SQLite, expose the reason, and continue accepting source writes.
func TestDatabaseLifecycleFailedTargetRecheckStaysOnSQLite(t *testing.T) {
	deployment := startLifecycleDeployment(t,
		[]string{"sqlite", "postgres"},
		nil,
	)
	baseURL := deployment.baseURL
	assertLifecycleBackend(t, baseURL, "sqlite")

	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":5150}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})

	target := "postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable"
	targetBody, _ := json.Marshal(map[string]string{"dsn": target})
	runMigrationGates(t, baseURL, targetBody)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	out, err := deployment.compose(ctx, "exec", "--no-TTY", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "-U", "loomarr", "-d", "loomarr",
		"-c", "CREATE TABLE changed_after_preflight (id integer)")
	cancel()
	if err != nil {
		t.Fatalf("change target after preflight: %v\n%s", err, out)
	}

	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/migrate", targetBody)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	failure, err := awaitDatabaseFailure(baseURL, 60*time.Second)
	if err != nil {
		t.Fatalf("failed target recheck did not recover SQLite: %v", err)
	}
	if !strings.Contains(strings.ToLower(failure), "target is empty") {
		t.Fatalf("migration failure = %q, want target-is-empty explanation", failure)
	}
	assertLifecycleTitle(t, baseURL, 5150)
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/titles",
		[]byte(`{"mediaType":"movie","tmdbId":5151}`))
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	assertLifecycleTitle(t, baseURL, 5151)
}

// TestDatabaseLifecycleDrainsInflightWritesBeforeCopy defines the concurrent
// write contract at the HTTP boundary. A request whose body is already being
// handled may finish; Shutdown closes admission to new requests and waits for
// that request before the read-only SQLite snapshot opens. The accepted row must
// therefore appear on Postgres rather than being stranded on the old source.
func TestDatabaseLifecycleDrainsInflightWritesBeforeCopy(t *testing.T) {
	directPort := reservePort(t)
	override := filepath.Join(t.TempDir(), "compose.direct.yaml")
	overrideBody := fmt.Sprintf(`services:
  loomarr:
    ports:
      - "127.0.0.1:%d:8080"
`, directPort)
	if err := os.WriteFile(override, []byte(overrideBody), 0o600); err != nil {
		t.Fatalf("write direct lifecycle probe override: %v", err)
	}
	deployment := startLifecycleDeployment(t,
		[]string{"sqlite", "postgres"},
		[]string{override},
	)
	baseURL := deployment.baseURL
	directURL := fmt.Sprintf("http://127.0.0.1:%d", directPort)
	assertLifecycleBackend(t, baseURL, "sqlite")

	target := "postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable"
	targetBody, _ := json.Marshal(map[string]string{"dsn": target})
	runMigrationGates(t, baseURL, targetBody)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", directPort), 5*time.Second)
	if err != nil {
		t.Fatalf("open in-flight write connection: %v", err)
	}
	defer func() { _ = conn.Close() }()
	writeBody := []byte(`{"mediaType":"movie","tmdbId":6262}`)
	split := len(writeBody) / 2
	headers := fmt.Sprintf("POST /v1/titles HTTP/1.1\r\nHost: 127.0.0.1\r\nAuthorization: Bearer lifecycle-test-token\r\nX-Loomarr-Csrf: 1\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(writeBody))
	if _, err := conn.Write(append([]byte(headers), writeBody[:split]...)); err != nil {
		t.Fatalf("start in-flight write: %v", err)
	}
	// The partial body keeps this request active inside the real server while the
	// separate migration request asks the generation to drain.
	time.Sleep(200 * time.Millisecond)

	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/migrate", targetBody)
	decodeLifecycleResponse(t, response, http.StatusOK, &struct{}{})
	if err := awaitUnavailable(directURL, 5*time.Second); err != nil {
		t.Fatalf("migration did not close admission while draining: %v", err)
	}

	if _, err := conn.Write(writeBody[split:]); err != nil {
		t.Fatalf("finish in-flight write during drain: %v", err)
	}
	writeResponse, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("read drained write response: %v", err)
	}
	_ = writeResponse.Body.Close()
	if writeResponse.StatusCode != http.StatusOK {
		t.Fatalf("in-flight write during drain = %d, want 200", writeResponse.StatusCode)
	}

	if err := awaitDatabaseBackend(baseURL, "postgres", 90*time.Second); err != nil {
		t.Fatalf("drained migration did not restart on Postgres: %v", err)
	}
	assertLifecycleTitle(t, baseURL, 6262)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve host port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func composeEnvironment(project, version string, port int) []string {
	blocked := map[string]bool{
		"COMPOSE_PROJECT_NAME": true,
		"DATABASE_URL":         true,
	}
	env := make([]string, 0, len(os.Environ())+12)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	return append(env,
		"COMPOSE_PROJECT_NAME="+project,
		"LOOMARR_VERSION="+version,
		"LOOMARR_HTTP_BIND=127.0.0.1",
		fmt.Sprintf("LOOMARR_HTTP_PORT=%d", port),
		"SERVER_PUBLIC_URL=http://loomarr:8080",
		"API_TOKEN=lifecycle-test-token",
		"LIBRARY_FLAVOR=",
		"LIBRARY_URL=",
		"LIBRARY_TOKEN=",
		"SEERR_URL=",
		"SEERR_API_KEY=",
		"TUNARR_URL=",
		"LLM_MODEL=",
		"TMDB_API_KEY=",
	)
}

func awaitReady(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	var last string
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/readyz")
		if err == nil {
			last = resp.Status
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		} else {
			last = err.Error()
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for /readyz (last result: %s)", last)
}

func lifecycleRequest(t *testing.T, method, url string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer lifecycle-test-token")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func lifecycleCookieRequest(t *testing.T, method, url string, body []byte, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s with session: %v", method, url, err)
	}
	return resp
}

func decodeLifecycleResponse(t *testing.T, resp *http.Response, want int, out any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		var problem map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&problem)
		t.Fatalf("response status = %d, want %d: %v", resp.StatusCode, want, problem)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertLifecycleBackend(t *testing.T, baseURL, want string) {
	t.Helper()
	resp := lifecycleRequest(t, http.MethodGet, baseURL+"/v1/system/database", nil)
	var status struct {
		Backend string `json:"backend"`
	}
	decodeLifecycleResponse(t, resp, http.StatusOK, &status)
	if status.Backend != want {
		t.Fatalf("database backend = %q, want %s", status.Backend, want)
	}
}

func assertLifecycleTitle(t *testing.T, baseURL string, tmdbID int) {
	t.Helper()
	path := fmt.Sprintf("%s/v1/titles/movie:tmdb:%d", baseURL, tmdbID)
	resp := lifecycleRequest(t, http.MethodGet, path, nil)
	var title struct {
		Key   string `json:"key"`
		State string `json:"state"`
	}
	decodeLifecycleResponse(t, resp, http.StatusOK, &title)
	wantKey := fmt.Sprintf("movie:tmdb:%d", tmdbID)
	if title.Key != wantKey || title.State != "wanted" {
		t.Fatalf("title after lifecycle = %+v, want key %s state wanted", title, wantKey)
	}
}

func assertLifecycleTitleMissing(t *testing.T, baseURL string, tmdbID int) {
	t.Helper()
	path := fmt.Sprintf("%s/v1/titles/movie:tmdb:%d", baseURL, tmdbID)
	resp := lifecycleRequest(t, http.MethodGet, path, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("title %d after rollback = %d, want 404", tmdbID, resp.StatusCode)
	}
}

func awaitDatabaseBackend(baseURL, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/system/database", nil)
		req.Header.Set("Authorization", "Bearer lifecycle-test-token")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			var status struct {
				Backend string `json:"backend"`
			}
			if resp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp.Body).Decode(&status)
			}
			_ = resp.Body.Close()
			last = fmt.Sprintf("status %d, backend %q", resp.StatusCode, status.Backend)
			if resp.StatusCode == http.StatusOK && status.Backend == want {
				return nil
			}
		} else {
			last = err.Error()
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for backend %s (last result: %s)", want, last)
}

func seedLargeSQLiteFixture(t *testing.T, databasePath string, titles int) string {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+databasePath, true)
	if err != nil {
		t.Fatalf("create migrated SQLite fixture: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close migrated SQLite fixture: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+databasePath+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("open SQLite fixture for bulk seed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin SQLite fixture seed: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO titles (key, title_json, state) VALUES (?, ?, 'wanted')`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare SQLite fixture seed: %v", err)
	}
	for id := 0; id < titles; id++ {
		key := fmt.Sprintf("movie:tmdb:%d", id)
		title := fmt.Sprintf(`{"mediaType":"movie","tmdbId":%d,"name":"Lifecycle fixture %d"}`, id, id)
		if _, err := stmt.Exec(key, title); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			t.Fatalf("seed SQLite fixture title %d: %v", id, err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close SQLite fixture statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit SQLite fixture: %v", err)
	}
	var version string
	if err := db.QueryRow(`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err != nil {
		t.Fatalf("read SQLite fixture schema version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close bulk-seeded SQLite fixture: %v", err)
	}
	return version
}

func runMigrationGates(t *testing.T, baseURL string, targetBody []byte) {
	t.Helper()
	response := lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/preflight", targetBody)
	var preflight struct {
		Passed bool `json:"passed"`
	}
	decodeLifecycleResponse(t, response, http.StatusOK, &preflight)
	if !preflight.Passed {
		t.Fatal("Postgres target did not pass preflight")
	}
	response = lifecycleRequest(t, http.MethodPost, baseURL+"/v1/system/database/backup", nil)
	var backup struct {
		Bytes int64 `json:"bytes"`
	}
	decodeLifecycleResponse(t, response, http.StatusOK, &backup)
	if backup.Bytes == 0 {
		t.Fatal("pre-migration backup is empty")
	}
}

func awaitPostgresTable(
	compose func(context.Context, ...string) ([]byte, error),
	table string,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	var last string
	query := fmt.Sprintf("SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public' AND tablename = '%s'", table)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := compose(ctx, "exec", "--no-TTY", "postgres", "psql",
			"-U", "loomarr", "-d", "loomarr", "-Atqc", query)
		cancel()
		last = strings.TrimSpace(string(out))
		if err == nil && last == "1" {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for Postgres table %s (last result: %q)", table, last)
}

func postgresScalar(
	t *testing.T,
	compose func(context.Context, ...string) ([]byte, error),
	query string,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := compose(ctx, "exec", "--no-TTY", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "-U", "loomarr", "-d", "loomarr", "-Atqc", query)
	if err != nil {
		t.Fatalf("query migrated Postgres fidelity oracle: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

type lifecycleDeployment struct {
	baseURL string
	compose func(context.Context, ...string) ([]byte, error)
}

func startLifecycleDeployment(t *testing.T, profiles, extraFiles []string) lifecycleDeployment {
	t.Helper()
	root := repositoryRoot(t)
	port := reservePort(t)
	project := fmt.Sprintf("loomarr-db-lifecycle-%d", time.Now().UnixNano())
	env := composeEnvironment(project, lifecycleImageVersion, port)
	base := []string{
		"compose", "--project-name", project,
		"--file", filepath.Join(root, "docker", "compose.yaml"),
	}
	for _, file := range extraFiles {
		base = append(base, "--file", file)
	}
	for _, profile := range profiles {
		base = append(base, "--profile", profile)
	}
	compose := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
		cmd.Dir = root
		cmd.Env = env
		return cmd.CombinedOutput()
	}

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "docker", "build",
		"--tag", "ghcr.io/loomarr/loomarr:"+lifecycleImageVersion,
		"--build-arg", "VERSION="+lifecycleImageVersion,
		root,
	)
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build shipped Loomarr image: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if out, err := compose(ctx, "down", "--volumes", "--remove-orphans"); err != nil {
			t.Errorf("clean lifecycle Compose project: %v\n%s", err, out)
		}
	})
	startCtx, cancelStart := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancelStart()
	if out, err := compose(startCtx, "up", "--detach"); err != nil {
		t.Fatalf("start lifecycle deployment: %v\n%s", err, out)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := awaitReady(baseURL, 45*time.Second); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		logs, _ := compose(ctx, "logs", "--no-color", "loomarr", "postgres")
		t.Fatalf("lifecycle deployment never became ready: %v\n%s", err, logs)
	}
	return lifecycleDeployment{baseURL: baseURL, compose: compose}
}

func awaitDatabaseFailure(baseURL string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/system/database", nil)
		req.Header.Set("Authorization", "Bearer lifecycle-test-token")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			var status struct {
				Backend string `json:"backend"`
				Phase   string `json:"phase"`
				Error   string `json:"error"`
			}
			if resp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp.Body).Decode(&status)
			}
			_ = resp.Body.Close()
			last = fmt.Sprintf("status %d, backend %q, phase %q, error %q",
				resp.StatusCode, status.Backend, status.Phase, status.Error)
			if resp.StatusCode == http.StatusOK && status.Backend == "sqlite" && status.Phase == "failed" {
				return status.Error, nil
			}
		} else {
			last = err.Error()
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out waiting for failed SQLite recovery (last result: %s)", last)
}

func awaitUnavailable(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/v1/readyz")
		if err != nil {
			return nil
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status != http.StatusOK {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("endpoint continued accepting requests throughout migration drain")
}
