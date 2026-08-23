package settings

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// memSecretStore is an in-memory SecretStore for the lifecycle tests. It persists
// across "restarts" (a fresh NewSecrets over the same store) to prove idempotency.
type memSecretStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemSecretStore() *memSecretStore { return &memSecretStore{m: map[string]string{}} }

func (s *memSecretStore) Get(_ context.Context, k string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memSecretStore) Set(_ context.Context, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

func noEnv(string) (string, bool) { return "", false }

// Generation is idempotent across restarts (config-design §4, §10): a second
// NewSecrets over the same store reads back the SAME values, never re-mints.
func TestSecrets_IdempotentAcrossRestarts(t *testing.T) {
	store := newMemSecretStore()
	ctx := context.Background()

	s1, err := NewSecrets(ctx, store, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewSecrets(ctx, store, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range allGenerated() {
		if s1.Value(g) == "" {
			t.Errorf("%s not generated", g)
		}
		if s1.Value(g) != s2.Value(g) {
			t.Errorf("%s not idempotent: %q != %q", g, s1.Value(g), s2.Value(g))
		}
	}
}

// An env-supplied secret wins and is NOT persisted (it's pinned — config-design §4).
func TestSecrets_EnvPinWinsNotPersisted(t *testing.T) {
	store := newMemSecretStore()
	env := func(k string) (string, bool) {
		if k == "API_TOKEN" {
			return "operator-pinned-token", true
		}
		return "", false
	}
	s, err := NewSecrets(context.Background(), store, env)
	if err != nil {
		t.Fatal(err)
	}
	if s.Value(SecretAPI) != "operator-pinned-token" {
		t.Errorf("env pin should win, got %q", s.Value(SecretAPI))
	}
	if _, ok, _ := store.Get(context.Background(), SecretAPI.dbKey()); ok {
		t.Error("env-pinned secret must NOT be persisted to the DB")
	}
}

// Regenerate rotates the value; an env-pinned secret refuses (config-design §4).
func TestSecrets_Regenerate(t *testing.T) {
	store := newMemSecretStore()
	ctx := context.Background()
	s, _ := NewSecrets(ctx, store, noEnv)
	old := s.Value(SecretPlayout)
	fresh, err := s.Regenerate(ctx, SecretPlayout)
	if err != nil {
		t.Fatal(err)
	}
	if fresh == old || s.Value(SecretPlayout) != fresh {
		t.Errorf("regenerate did not rotate: old=%q fresh=%q live=%q", old, fresh, s.Value(SecretPlayout))
	}

	// Env-pinned → refuse.
	env := func(k string) (string, bool) {
		if k == "API_TOKEN" {
			return "pinned", true
		}
		return "", false
	}
	sp, _ := NewSecrets(ctx, newMemSecretStore(), env)
	if _, err := sp.Regenerate(ctx, SecretAPI); err == nil {
		t.Error("regenerating an env-pinned secret should error")
	}
	if _, err := s.Regenerate(ctx, GeneratedSecret("unknown")); err == nil {
		t.Error("regenerating an unknown generated secret should error")
	}
}

func TestSecrets_CurrentObservesRotationFromAnotherReplica(t *testing.T) {
	store := newMemSecretStore()
	ctx := context.Background()
	first, err := NewSecrets(ctx, store, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSecrets(ctx, store, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	old := second.Value(SecretPlayout)
	fresh, err := first.Regenerate(ctx, SecretPlayout)
	if err != nil {
		t.Fatal(err)
	}
	if second.Value(SecretPlayout) != old {
		t.Fatal("a process-local cache changed without a durable read")
	}
	got, err := second.Current(ctx, SecretPlayout)
	if err != nil {
		t.Fatal(err)
	}
	if got != fresh || second.Value(SecretPlayout) != fresh {
		t.Fatalf("Current = %q, cache = %q, want rotated %q", got, second.Value(SecretPlayout), fresh)
	}
}

func TestSecrets_CurrentFailsClosedWhenDurableValueDisappears(t *testing.T) {
	store := newMemSecretStore()
	ctx := context.Background()
	secrets, err := NewSecrets(ctx, store, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	delete(store.m, SecretPlayout.dbKey())
	store.mu.Unlock()
	if _, err := secrets.Current(ctx, SecretPlayout); err == nil {
		t.Fatal("Current minted or returned a cached value after the durable secret disappeared")
	}
}

// THE log-grep redaction gate (config-design §4, §10): a known secret value fed to
// the Redactor never appears in captured log output, whether logged as a message,
// an attribute, or embedded in a larger string.
func TestRedactor_SecretNeverInLogs(t *testing.T) {
	secret := "sk-super-secret-value-abc123XYZ"
	var buf bytes.Buffer
	r := NewRedactor()
	r.Set([]string{secret})
	log := slog.New(r.Handler(slog.NewTextHandler(&buf, nil)))

	// Every way a careless caller might leak it:
	log.Info("connecting with token " + secret)
	log.Info("auth", "token", secret)
	log.Info("auth", "url", "http://x?apikey="+secret+"&y=1")
	log.Error("failed", "detail", "response body contained "+secret)
	log.With("bound_token", secret).Info("pre-bound attribute")

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("SECRET LEAKED into logs:\n%s", out)
	}
	if !strings.Contains(out, "‹redacted›") {
		t.Error("expected a redaction marker in the output")
	}
}

func TestRedactor_AppliesCurrentSecretsToPreBoundAndGroupedAttributes(t *testing.T) {
	const secret = "secret-discovered-after-logger-binding"
	var buf bytes.Buffer
	r := NewRedactor()
	log := slog.New(r.Handler(slog.NewJSONHandler(&buf, nil))).
		With("bound", secret).
		WithGroup("request").
		With("nested", slog.GroupValue(slog.String("credential", secret)))

	// Settings refresh can discover or rotate a secret after a long-lived subsystem logger was
	// constructed. Redaction must happen when the record is handled, not when With captured it.
	r.Set([]string{secret})
	log.Info("ready")
	out := buf.String()
	if strings.Contains(out, secret) || !strings.Contains(out, "‹redacted›") {
		t.Fatalf("current redaction did not cover bound/grouped attrs: %s", out)
	}
}

// The generated tokens feed the Redactor (config-design §4).
func TestSecrets_FeedRedactor(t *testing.T) {
	s, err := NewSecrets(context.Background(), newMemSecretStore(), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	vals := s.RedactionValues()
	if len(vals) != len(allGenerated()) {
		t.Fatalf("expected %d generated secret values, got %d", len(allGenerated()), len(vals))
	}
	var buf bytes.Buffer
	r := NewRedactor()
	r.Set(vals)
	log := slog.New(r.Handler(slog.NewTextHandler(&buf, nil)))
	log.Info("token is " + s.Value(SecretAPI))
	if strings.Contains(buf.String(), s.Value(SecretAPI)) {
		t.Fatalf("generated API token leaked:\n%s", buf.String())
	}
}

// A short value below the floor is not tracked (avoids redacting everything).
func TestRedactor_ShortValueFloor(t *testing.T) {
	var buf bytes.Buffer
	r := NewRedactor()
	r.Set([]string{"ab"}) // below the 8-char floor
	log := slog.New(r.Handler(slog.NewTextHandler(&buf, nil)))
	log.Info("about")
	if strings.Contains(buf.String(), "‹redacted›") {
		t.Error("a sub-floor value should not be redacted")
	}
}

// preview masks a secret to its last 4 chars (config-design §4).
func TestPreview_MasksToLast4(t *testing.T) {
	if got := preview("abcdef1234"); got != "…1234" {
		t.Errorf("preview = %q, want …1234", got)
	}
	if got := preview(""); got != "" {
		t.Errorf("empty preview = %q, want empty", got)
	}
}

// V4's gate: the playout token is a real secret, so it must reach the Redactor like
// every other one. It is generated by All() rather than named anywhere in the redaction
// path, so this asserts the wiring rather than the mechanism — a secret added to the
// enum but forgotten in allGenerated() would leak, and nothing else would notice.
func TestPlayoutTokenIsRedactable(t *testing.T) {
	found := false
	for _, g := range allGenerated() {
		if g == SecretPlayout {
			found = true
		}
	}
	if !found {
		t.Fatal("playout_token is not in allGenerated() — it would never reach the Redactor (§4)")
	}
	// But it is NOT the API token: that one is break-glass admin with full authority.
	if SecretPlayout == SecretAPI {
		t.Error("playout_token must be distinct from api_token (device auth vs admin auth, §11)")
	}
}
