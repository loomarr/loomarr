package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The bootstrap FILE tier (config-design §1, added by V5).
//
// Bootstrap keys are env-only for a precise reason: they are needed *before the
// database opens*, so the registry — which lives in that database — cannot resolve
// them. That reasoning is sound and unchanged. What it did not anticipate is the
// wizard needing to WRITE one.
//
// The Database step (§13) asks "SQLite or PostgreSQL?" and must persist the answer.
// It cannot write to the database it is configuring, and it cannot set an env var on
// a running process in a way that survives a restart. A file beside the database is
// the only writable store available before the database exists.
//
// So the tier is deliberately NARROW:
//
//	env > file > default          — bootstrap keys ONLY
//	env > database > default      — every app-managed setting, unchanged
//
//   - Env still wins. A GitOps operator who pins DATABASE_URL keeps deterministic
//     control, and the wizard must not be able to silently override a pin — it
//     reports the key as pinned instead, exactly like the registry does.
//   - The file holds ONLY bootstrap keys. It is not a second settings store, and
//     an app-managed key appearing here is a bug, not a feature: it would create
//     two places to look for one answer, which is the drift the registry exists to
//     prevent.
//   - Absent file = the current behaviour, exactly. This tier adds a lookup, never
//     a requirement.

// BootstrapFileName is the file's fixed name. It lives beside the SQLite database in
// the data directory, so the documented `-v loomarr-data:/data` volume carries it and
// a backup that captures /data captures the bootstrap answer too.
const BootstrapFileName = "bootstrap.json"

// bootstrapFile is the on-disk shape. Deliberately a flat map of the SAME env-var
// names the Config struct tags use, rather than a typed struct: the file is written
// by the wizard and read here, and keeping one vocabulary (the env name) means an
// operator reading the file, the docs, and `docker run -e` all see the same token.
type bootstrapFile struct {
	// Values maps env-var name → value, e.g. {"DATABASE_URL": "postgres://…"}.
	Values map[string]string `json:"values"`
}

// bootstrapKeys is the closed set the file may carry — the env-var names of the
// bootstrap Config fields. A key outside this set is REJECTED rather than ignored:
// silently dropping it would let someone believe they had configured something.
var bootstrapKeys = map[string]bool{
	"LISTEN_ADDR":  true,
	"LOG_LEVEL":    true,
	"TZ":           true,
	"DATABASE_URL": true,
	"AUTO_MIGRATE": true,
}

// LoadBootstrapFile reads the bootstrap file from dir, returning its values. A missing
// file is not an error — it is the normal state for an env-configured or first-run
// install. A malformed or out-of-scope file IS an error: this runs before the database
// opens, so failing loudly at boot beats starting with a configuration the operator
// thinks is applied and isn't.
func LoadBootstrapFile(dir string) (map[string]string, error) {
	if dir == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, BootstrapFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // normal: no file yet
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", BootstrapFileName, err)
	}
	var bf bootstrapFile
	if err := json.Unmarshal(raw, &bf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", BootstrapFileName, err)
	}
	for k := range bf.Values {
		if !bootstrapKeys[strings.ToUpper(k)] {
			// Named explicitly so the operator can see WHICH key is wrong, and told
			// where it belongs — an app-managed key here is a category error, not a
			// typo, and pointing at Settings is more useful than "unknown key".
			return nil, fmt.Errorf(
				"%s carries %q, which is not a bootstrap key — app-managed settings belong in Settings (env > database > default), not this file",
				BootstrapFileName, k)
		}
	}
	return bf.Values, nil
}

// WriteBootstrapFile persists bootstrap values to dir, replacing the file. Used by the
// wizard's Database step (§13) — the one writer. Rejects non-bootstrap keys for the
// same reason the reader does.
//
// Written atomically (temp file + rename) because this file decides where the database
// IS: a half-written one on a crashed write would leave the next boot unable to find
// its own data, which is the worst failure this file could have.
func WriteBootstrapFile(dir string, values map[string]string) error {
	if dir == "" {
		return errors.New("no data directory configured")
	}
	for k := range values {
		if !bootstrapKeys[strings.ToUpper(k)] {
			return fmt.Errorf("%q is not a bootstrap key", k)
		}
	}
	raw, err := json.MarshalIndent(bootstrapFile{Values: values}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp := filepath.Join(dir, BootstrapFileName+".tmp")
	// 0600: DATABASE_URL routinely carries a password. The file sits in the data
	// volume next to the database, which is already credential-bearing, but there is
	// no reason to widen it.
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, BootstrapFileName)); err != nil {
		return fmt.Errorf("replace %s: %w", BootstrapFileName, err)
	}
	return nil
}

// ConventionalDataDir is the well-known data directory — the documented
// `-v loomarr-data:/data` volume, and where the bootstrap file is WRITTEN.
const ConventionalDataDir = "/data"

// BootstrapDirsFor returns the directories to search for the bootstrap file, in
// precedence order (first hit wins).
//
// ⚠ This exists because the file's location was scheme-DEPENDENT, and a database
// migration changes the scheme. `DataDirFor` returns the SQLite file's own directory
// for `sqlite://` but the conventional /data for `postgres://` — so migrating
// SQLite→Postgres moved where the next boot LOOKS for the file. With the database
// anywhere other than /data, the file recording the switch was written beside the
// SQLite database and then never read again: the app would boot back onto SQLite,
// having apparently succeeded at migrating. The switch would silently undo itself.
//
// Co-location with the data volume was always the actual intent (that is what makes a
// backup of /data carry the bootstrap answer too); the scheme-dependence was incidental
// to expressing it. So the READ side searches both — the database's own directory first,
// because an operator who pinned a SQLite path meant that one — and the WRITE side
// (`DataDirFor`) is unchanged. An install whose file sits beside a non-default SQLite
// path keeps working exactly as before; nothing has to move.
func BootstrapDirsFor(databaseURL string) []string {
	dir := DataDirFor(databaseURL)
	if dir == ConventionalDataDir {
		return []string{ConventionalDataDir}
	}
	return []string{dir, ConventionalDataDir}
}

// loadBootstrapSearch reads the first bootstrap file found across dirs. A malformed file
// still fails the boot (LoadBootstrapFile's contract) rather than falling through to the
// next directory — a file that exists and is wrong is an operator error to surface, not
// a reason to quietly use a different one.
func loadBootstrapSearch(dirs []string) (map[string]string, error) {
	for _, dir := range dirs {
		values, err := LoadBootstrapFile(dir)
		if err != nil {
			return nil, err
		}
		if values != nil {
			return values, nil
		}
	}
	return nil, nil
}

// PinnedByEnv reports whether an env var is set in the process environment. The wizard
// asks before offering to write a key: an env pin WINS over the file, so offering an
// editable field for a pinned key would promise something the next boot would silently
// undo. Same contract the registry's `pinned` provenance already gives the Settings UI.
func PinnedByEnv(envVar string) bool {
	_, ok := os.LookupEnv(envVar)
	return ok
}
