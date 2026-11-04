package config

import (
	"fmt"
	"os"
	"strings"
)

// ReadSecret returns the contents of a Docker-secrets style file with surrounding whitespace
// removed. It is the one reader behind every <VAR>_FILE: the metrics token, registry-setting
// secrets, and the installation key. Errors name the path and never the contents.
func ReadSecret(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied config, by design
	if err != nil {
		return "", fmt.Errorf("read secret file %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// LookupEnv reports a process environment variable. Code outside this package that must
// consult the environment at runtime (an injected default for a lookup seam) calls this, so
// os.LookupEnv stays confined to internal/config.
func LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }
