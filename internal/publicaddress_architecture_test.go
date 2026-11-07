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

// "Is this address public?" has exactly one answer: httpx.IsPublicIP (and the pinned
// httpx.PublicDialContext built on it). Five hand-rolled copies once disagreed, and none of them
// refused 100.64.0.0/10, so a member-supplied URL could make the server fetch a tailnet host.
//
// The net.IP / netip.Addr range predicates are how a new copy starts, so calling one outside
// internal/httpx fails here. The guard reads production source with go/parser over
// loomarrPackages (never `go list`, which the test cache cannot track — see importgraph_test.go).
// It is syntactic: it cannot see receiver types, so a method of the same name on another type is
// also flagged and belongs in the allowlist with the reason.

const publicAddressHome = "internal/httpx/"

var addressRangePredicates = map[string]bool{
	"IsPrivate": true, "IsLoopback": true, "IsLinkLocalUnicast": true, "IsLinkLocalMulticast": true,
	"IsInterfaceLocalMulticast": true, "IsGlobalUnicast": true, "IsUnspecified": true, "IsMulticast": true,
}

// addressPredicateAllowlist maps a repo-relative file to why its address-range checks are not an
// "is this safe to fetch?" decision. Anything that is an SSRF gate uses httpx instead.
var addressPredicateAllowlist = map[string]string{
	"internal/llm/openai.go": "classifies an operator-configured base URL as self-hosted (an ALLOW rule that " +
		"decides which request fields to send), including CGNAT on purpose; it gates no fetch",
	"internal/openroutercatalog/openrouter.go": "permits a plain-HTTP base URL only for loopback; it gates no fetch to a stranger-supplied host",
	"internal/filler/location/index.go": "geo-IP data-range guard that skips lookup of addresses with no country; " +
		"not an SSRF gate (follow-up: adopt httpx.IsPublicIP so the two cannot drift)",
}

// addressPredicateCalls returns "Method@line" for every zero-argument call of an address-range
// predicate in src.
func addressPredicateCalls(t *testing.T, filename string, src any) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && addressRangePredicates[sel.Sel.Name] {
			found = append(found, sel.Sel.Name+"@"+strconv.Itoa(fset.Position(call.Pos()).Line))
		}
		return true
	})
	return found
}

// productionAddressPredicateUses maps each repo-relative production file outside internal/httpx
// to its address-range predicate calls.
func productionAddressPredicateUses(t *testing.T, pkgs map[string]*build.Package) map[string][]string {
	t.Helper()
	uses := map[string][]string{}
	for importPath, pkg := range pkgs {
		rel := strings.TrimPrefix(importPath, modulePath+"/")
		for _, name := range pkg.GoFiles {
			file := filepath.ToSlash(filepath.Join(rel, name))
			if strings.HasPrefix(file, publicAddressHome) {
				continue
			}
			if calls := addressPredicateCalls(t, filepath.Join(pkg.Dir, name), nil); len(calls) > 0 {
				uses[file] = calls
			}
		}
	}
	return uses
}

func TestAddressRangePredicatesLiveOnlyInHTTPX(t *testing.T) {
	uses := productionAddressPredicateUses(t, loomarrPackages(t))
	var files []string
	for file := range uses {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if _, ok := addressPredicateAllowlist[file]; !ok {
			t.Errorf("%s calls %s — use httpx.IsPublicIP / httpx.PublicDialContext for \"may the server "+
				"connect here?\"; if this is not an SSRF decision, add the file to addressPredicateAllowlist with the reason",
				file, strings.Join(uses[file], ", "))
		}
	}
}

func TestAddressPredicateAllowlistIsCurrent(t *testing.T) {
	uses := productionAddressPredicateUses(t, loomarrPackages(t))
	for file, reason := range addressPredicateAllowlist {
		if reason == "" {
			t.Errorf("%s is allowlisted without a reason", file)
		}
		if len(uses[file]) == 0 {
			t.Errorf("%s no longer calls an address-range predicate — drop it from addressPredicateAllowlist", file)
		}
	}
}

func TestAddressPredicateScanFlagsEveryHandRolledForm(t *testing.T) {
	src := `package p

import ("net"; "net/netip")

func a(ip net.IP) bool       { return ip.IsPrivate() }
func b(ip net.IP) bool       { return ip.IsLoopback() || ip.IsLinkLocalUnicast() }
func c(ip netip.Addr) bool   { return ip.Unmap().IsGlobalUnicast() }
func d(ip net.IP) bool       { return net.ParseIP("1.2.3.4").IsMulticast() }
func e(s fmtLike) string     { return s.String() }
func f(ip net.IP) bool       { return ip.IsLoopback }
`
	got := strings.Join(addressPredicateCalls(t, "p.go", src), " ")
	want := "IsPrivate@5 IsLoopback@6 IsLinkLocalUnicast@6 IsGlobalUnicast@7 IsMulticast@8"
	if got != want {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}
