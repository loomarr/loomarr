package internal_test

import (
	"bufio"
	"flag"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A comment says what the code is and why, in the present tense. A comment that narrates how the
// code got here ("re-score #4 moved this", "since write version 51 it...", "this now reads the
// table") is true for one commit and wrong for every one after it, and the history it carries
// already lives in git, the PR and docs/. This guard fails a non-test Go file that gains such a
// comment.
//
// It is a ratchet, not a clean slate: testdata/history-comments.txt holds one "path count" line
// per file that already has hits. A file may not gain a hit, and a file that loses one must lower
// its line (so the number only ever falls). Fix a hit by rewording the comment to the present
// tense, or by moving the history to the commit message, then lower the ledger with
//
//	go test ./internal/ -run TestCommentsDoNotNarrateHistory -lower-history-ledger
//
// which only ever lowers or deletes lines.
//
// The scan is syntactic, over production source from loomarrPackages (see importgraph_test.go),
// and reads comments only, never string literals. Text inside double quotes or backticks on one
// line is an example or a quotation, not the comment speaking, and is ignored. The rules prefer
// precision to reach: a sentence a rule wrongly flags is fixed in the rule's pattern, never by an
// exemption for the line.

const historyLedger = "testdata/history-comments.txt"

var lowerHistoryLedger = flag.Bool("lower-history-ledger", false,
	"rewrite "+historyLedger+" to the current counts where they are lower")

// quotedSpan is a quoted or backticked span, which may wrap onto the next line. The length bound
// keeps a stray quote mark (an inch sign) from swallowing the comment after it.
var quotedSpan = regexp.MustCompile("\"[^\"]{0,160}\"|`[^`]{0,160}`")

// withoutQuotes blanks every quoted span, keeping its newlines so line numbers hold.
func withoutQuotes(comment string) string {
	return quotedSpan.ReplaceAllStringFunc(comment, func(span string) string {
		return `""` + strings.Repeat("\n", strings.Count(span, "\n"))
	})
}

// historyRules are the narrations the guard recognises. Each pattern is matched against one
// comment line.
var historyRules = []struct {
	name string
	re   *regexp.Regexp
}{
	{
		// A numbered re-score round, "re-score #4" or "re-score 11-12". The verb ("a re-pick
		// re-scores the entry") is ordinary prose and never matches.
		name: "re-score round",
		re:   regexp.MustCompile(`(?i)\bre-?scores?\s*(?:#\d+|\d{1,2}-\d{1,2}\b|round \d+|no\.?\s*\d+)`),
	},
	{
		// "Write version N" narrates which release changed a rule. What each version changed
		// lives in docs/ (the scanner's in docs/design/library.md), never in a comment.
		name: "write version N",
		re:   regexp.MustCompile(`(?i)\bwrite[- ]version\s+\d+`),
	},
	{
		// "now" as "as of this change": "is now the", "has now been", "now owns". It is a narrow set
		// on purpose: "where the clip is now" and "Run-now" are present tense and must not match.
		name: `"now" as of this change`,
		re: regexp.MustCompile(
			`\b(?:is|are) now (?:the|a|an|only|always|never|guaranteed|derived|final|owned|handled|checked|enforced|refused|recorded)\b` +
				`|\bhas now (?:been|moved|become|gone|paid|recorded)\b` +
				`|(?:^|[^-\w])now (?:owns|lives in|moved|goes through|reads|uses|returns|lands|carries|handles|records|resolves|refuses|rejects|keeps|no longer|only|also|takes|runs|calls|sits|treats|checks|lists|passes|gives|reuses|shares|writes|skips|waits|stops|starts|applies|accepts|serves|answers|asks|sends|emits|exposes|wraps|replaces|routes)\b`),
	},
	{
		// What a thing did before: "it used to be". The purpose sense ("used to key the record",
		// "the key it used to sign") does not open a clause with its subject and does not match.
		name: `"used to"`,
		re: regexp.MustCompile(
			`(?:^\s*(?://|/\*)?|[.;:(—] *)[^\w"]*(?:[A-Z][a-z]+|it|this|that|they) used to\b|(?i:\bused to (?:be|live|come|collapse|happen|have to)\b)`),
	},
	{
		// "formerly" and "previously" as narration. "A previously declined item" describes the
		// item's state and does not match.
		name: `"formerly" or "previously"`,
		re: regexp.MustCompile(
			`(?i)(?:[—;]|\s-)\s*(?:formerly|previously)\b|\b(?:formerly|previously) (?:was|were|had|did|lacked|lived)\b|\b(?:it|this|scheduler|code) (?:formerly|previously)\b`),
	},
	{
		// A change, an issue or a build used as a point in time: "as it was until #1674",
		// "since #1684", "until V38c". Citing an issue by number alone is allowed.
		name: "before or after a change",
		re: regexp.MustCompile(
			`\b(?:[Uu]ntil|[Ss]ince|[Bb]efore) (?:#\d+|V\d+[a-z]?\b|PR ?#?\d+)|\bwas\b[^.]{0,60}\buntil (?:#\d+|20\d\d)`),
	},
	{
		// A dated incident note. The date belongs in the commit or the issue.
		name: "dated note",
		re:   regexp.MustCompile(`\b20\d\d-[01]\d-[0-3]\d\b`),
	},
}

// historyComments returns "line: rule" for every comment line in src that narrates history.
func historyComments(t *testing.T, filename string, src any) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	// A generated file's comments are copied from its input (sqlc copies them from the .sql).
	if ast.IsGenerated(file) {
		return nil
	}
	var found []string
	for _, group := range file.Comments {
		// Quotes are blanked across the whole group, so a quotation that wraps stays quoted.
		var texts []string
		var lines []int
		for _, comment := range group.List {
			start := fset.Position(comment.Slash).Line
			for offset, text := range strings.Split(comment.Text, "\n") {
				texts = append(texts, text)
				lines = append(lines, start+offset)
			}
		}
		for i, text := range strings.Split(withoutQuotes(strings.Join(texts, "\n")), "\n") {
			for _, rule := range historyRules {
				if rule.re.MatchString(text) {
					found = append(found, strconv.Itoa(lines[i])+": "+rule.name)
				}
			}
		}
	}
	return found
}

func productionHistoryComments(t *testing.T, pkgs map[string]*build.Package) map[string][]string {
	t.Helper()
	hits := map[string][]string{}
	for importPath, pkg := range pkgs {
		rel := strings.TrimPrefix(importPath, modulePath+"/")
		for _, name := range pkg.GoFiles {
			if found := historyComments(t, filepath.Join(pkg.Dir, name), nil); len(found) > 0 {
				hits[filepath.ToSlash(filepath.Join(rel, name))] = found
			}
		}
	}
	return hits
}

func readHistoryLedger(t *testing.T) map[string]int {
	t.Helper()
	f, err := os.Open(historyLedger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	ledger := map[string]int{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, count, ok := strings.Cut(line, " ")
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if !ok || err != nil {
			t.Fatalf("%s: %q is not a \"path count\" line", historyLedger, line)
		}
		ledger[path] = n
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return ledger
}

// writeLoweredHistoryLedger rewrites the ledger keeping its header, lowering each line to the
// current count and deleting the lines that reach zero. It never adds a file or raises a count.
func writeLoweredHistoryLedger(t *testing.T, ledger map[string]int, hits map[string][]string) {
	t.Helper()
	old, err := os.ReadFile(historyLedger)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, line := range strings.SplitAfter(string(old), "\n") {
		if strings.HasPrefix(line, "#") {
			out.WriteString(line)
		}
	}
	var files []string
	for file, allowed := range ledger {
		ledger[file] = min(allowed, len(hits[file]))
		if ledger[file] == 0 {
			delete(ledger, file)
			continue
		}
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		out.WriteString(file + " " + strconv.Itoa(ledger[file]) + "\n")
	}
	if err := os.WriteFile(historyLedger, []byte(out.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCommentsDoNotNarrateHistory(t *testing.T) {
	hits := productionHistoryComments(t, loomarrPackages(t))
	ledger := readHistoryLedger(t)
	if *lowerHistoryLedger {
		writeLoweredHistoryLedger(t, ledger, hits)
	}
	var files []string
	for file := range hits {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		switch allowed := ledger[file]; {
		case len(hits[file]) > allowed:
			t.Errorf("%s has %d history-narrating comment(s), the ledger allows %d: line %s. Reword to the present "+
				"tense (what the code is and why), or move the history to the commit message. If this is the "+
				"version constant's own doc, name the constant ...Version.\nledger line if you are sure: %s %d",
				file, len(hits[file]), allowed, strings.Join(hits[file], "; line "), file, len(hits[file]))
		case len(hits[file]) < allowed:
			t.Errorf("%s now has %d hit(s) against a ledger of %d: lower its line in %s to %s %d",
				file, len(hits[file]), allowed, historyLedger, file, len(hits[file]))
		}
	}
	for file, allowed := range ledger {
		if _, ok := hits[file]; !ok {
			t.Errorf("%s has no history-narrating comments left (ledger says %d): delete its line from %s", file, allowed, historyLedger)
		}
	}
}

func TestHistoryCommentScanFlagsEveryForm(t *testing.T) {
	src := `package p

// A re-pick re-scores the entry, which is ordinary prose.
// re-score #4 moved this out of the resolver.
// Run-now goes through the queue, and the clip is now where it was.
// The pool is now the only owner of the cycle.
// Since write version 51 the key is lowercase.

// writeVersion changes when the output does. Write version 66 added the sidecar.
const writeVersion = 66

/* The reader
   now reads the cache first. */
func f() {}

// The title store used to resolve content, and a previously declined item.
// A phrase like "we used to watch" or the file "2019-06-01.mp4" is quoted.
// A detached channel no longer airs (#1965).
// Identity used to be the path.
// Preview agrees; previously each read its own.
// The scheduler previously lacked this memory.
// It was keyed by series until #1674.
// Measured on 2026-08-11.
// Verify with the key it used to sign (previously cached or not). A name such as "Lark
// 2023-11-02" wraps.
func g() {}
`
	got := strings.Join(historyComments(t, "p.go", src), " | ")
	want := `4: re-score round | 6: "now" as of this change | 7: write version N | 9: write version N | 13: "now" as of this change` +
		` | 19: "used to" | 20: "formerly" or "previously" | 21: "formerly" or "previously" | 22: before or after a change | 23: dated note`
	if got != want {
		t.Fatalf("flagged\n %s\nwant\n %s", got, want)
	}
}
