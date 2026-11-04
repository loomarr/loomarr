package internal_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Process environment access has one home: internal/config. Boot parsing takes an explicit
// environment map (config.LoadEnv), secrets come from config.ReadSecret, and code that must
// consult a variable at runtime goes through config.LookupEnv. A stray os.Getenv elsewhere is
// how a second, differently-precedenced reading of the same knob starts.
//
// The guard reads production source with go/parser over loomarrPackages (never `go list`, which
// the test cache cannot track — see importgraph_test.go). It is syntactic: it matches the
// selector `os.Getenv` / `os.LookupEnv` / `os.Setenv` / `os.Unsetenv` / `os.Environ` where `os`
// is the file's import of the standard "os" package.

const envAccessHome = "internal/config/"

var envAccessFuncs = map[string]bool{
	"Getenv": true, "LookupEnv": true, "Setenv": true, "Unsetenv": true, "Environ": true, "ExpandEnv": true, "Clearenv": true,
}

// envAccessAllowlist maps a repo-relative file (or a directory prefix ending in "/") to why it
// reads the environment itself.
var envAccessAllowlist = map[string]string{
	"cmd/demo-library/":  "standalone developer tool; flags default from the invoking shell's variables, never from a Loomarr bootstrap file",
	"cmd/image-bench/":   "standalone benchmark tool; flags default from the invoking shell's variables",
	"cmd/image-cert/":    "standalone certification tool; flags default from the invoking shell's variables",
	"cmd/release-notes/": "release tooling run in CI; reads its API key and options from the CI environment",
	"cmd/eval/":          "standalone diagnostic and certification tools; pass os.Getenv as their injected lookup",
	"cmd/dev-bootstrap/": "developer tooling; reads the repo root the dev-env script exports",
	"cmd/playout-bench/": "standalone benchmark tool; reads CI artifact and summary paths",
	"cmd/seed/":          "developer seeding tool; reads its pick list from the invoking shell",
	"cmd/loomarr/":       "terminal presentation only (NO_COLOR, COLUMNS); not Loomarr configuration",
	"project/evidence/":  "measurement programs for design evidence, run by hand from a shell; not part of the server",
	"internal/testkit/":  "test support that spawns helper processes and steers them through the environment",
}

func envAccessAllowed(file string) bool {
	if strings.HasPrefix(file, envAccessHome) {
		return true
	}
	for entry := range envAccessAllowlist {
		if file == entry || (strings.HasSuffix(entry, "/") && strings.HasPrefix(file, entry)) {
			return true
		}
	}
	return false
}

// envAccessCalls returns "os.Func@line" for every reference to a process-environment function of
// the standard "os" package in src (calls and function-value references such as
// `var lookup = os.LookupEnv`).
func envAccessCalls(t *testing.T, filename string, src any) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	osName := ""
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "os" {
			continue
		}
		osName = "os"
		if imp.Name != nil {
			osName = imp.Name.Name
		}
	}
	if osName == "" || osName == "_" || osName == "." {
		return nil
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !envAccessFuncs[sel.Sel.Name] {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == osName {
			found = append(found, "os."+sel.Sel.Name+"@"+strconv.Itoa(fset.Position(sel.Pos()).Line))
		}
		return true
	})
	return found
}

func productionEnvAccess(t *testing.T, pkgs map[string]*build.Package) map[string][]string {
	t.Helper()
	uses := map[string][]string{}
	for importPath, pkg := range pkgs {
		rel := strings.TrimPrefix(importPath, modulePath+"/")
		for _, name := range pkg.GoFiles {
			file := filepath.ToSlash(filepath.Join(rel, name))
			if calls := envAccessCalls(t, filepath.Join(pkg.Dir, name), nil); len(calls) > 0 {
				uses[file] = calls
			}
		}
	}
	return uses
}

func TestProcessEnvironmentIsReadOnlyInConfig(t *testing.T) {
	uses := productionEnvAccess(t, loomarrPackages(t))
	var files []string
	for file := range uses {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if !envAccessAllowed(file) {
			t.Errorf("%s uses %s — read configuration through internal/config (config.Load / config.LookupEnv / "+
				"config.ReadSecret); if it truly must read the process environment, add it to envAccessAllowlist with the reason",
				file, strings.Join(uses[file], ", "))
		}
	}
}

func TestEnvAccessAllowlistIsCurrent(t *testing.T) {
	uses := productionEnvAccess(t, loomarrPackages(t))
	for entry, reason := range envAccessAllowlist {
		if reason == "" {
			t.Errorf("%s is allowlisted without a reason", entry)
		}
		hit := false
		for file := range uses {
			if file == entry || (strings.HasSuffix(entry, "/") && strings.HasPrefix(file, entry)) {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("%s no longer reads the process environment — drop it from envAccessAllowlist", entry)
		}
	}
}

func TestEnvAccessScanFlagsEveryForm(t *testing.T) {
	src := `package p

import "os"

var lookup = os.LookupEnv

func a() string   { return os.Getenv("X") }
func b()          { _ = os.Setenv("X", "1") }
func c() []string { return os.Environ() }
func d() string   { h, _ := os.Hostname(); return h }
`
	got := strings.Join(envAccessCalls(t, "p.go", src), " ")
	want := "os.LookupEnv@5 os.Getenv@7 os.Setenv@8 os.Environ@9"
	if got != want {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}
