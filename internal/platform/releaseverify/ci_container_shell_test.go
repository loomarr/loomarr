package releaseverify

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Every first line below is valid shell. The reader must still see the command
// that follows it.
var shellQuotingLines = map[string]string{
	"plain":                              `v="plain"`,
	"escaped quote in substitution":      `v="$(printf '%s' "a\"b")"`,
	"double quote in single quotes":      `v="$(tr -d '"' </dev/null)"`,
	"quoted sed pattern":                 `v="$(sed -n 's/^c = "\([^"]*\)".*/\1/p' f)"`,
	"unquoted substitution":              `v=$(tr -d '"' < f)`,
	"nested substitution":                `v="a $(printf '%s' "$(tr -d '"' < f)") b"`,
	"single quotes in substitution":      `v="$(printf '(' )"`,
	"escaped quote":                      `v="a\"b" w=\'`,
	"subshell inside substitution":       `v="$( (tr -d '"') < f)"`,
	"case inside substitution":           `v="$(case x in x) printf '"' ;; esac)"`,
	"comment inside substitution":        "v=\"$(true # don't )\n)\"",
	"trailing comment":                   `true # don't "stop"`,
	"backticks":                          "v=`tr -d '\"' < f`",
	"backticks in double quotes":         "v=\"`tr -d '\"' < f`\"",
	"parameter length is not comment":    `v="$(echo ${#1} '"')"`,
	"ansi-c quoting":                     `v=$'it\'s'`,
	"arithmetic shift is not heredoc":    `v=$((1 << 2))`,
	"here-string":                        `cat <<< "it's"`,
	"quoted heredoc":                     "cat <<'EOF'\ndon't \"stop $(\nEOF",
	"expanding heredoc":                  "cat <<EOF\ndon't $(tr -d '\"' < f)\nEOF",
	"tab-stripped heredoc":               "cat <<-EOF\n\tit's\n\tEOF",
	"heredoc inside quoted substitution": "v=\"$(cat <<EOF\nit's )\nEOF\n)\"",
	"double quotes in expansion":         `v="${x:-"a b"}"`,
	"escaped quote in expansion":         `v="${x//\"/}"`,
	"quoted brace in expansion":          `v=${x#'}'}`,
	"substitution in expansion":          `v="${x:-$(tr -d '"' < f)}"`,
	"ansi-c after parameter":             `v=${x}$'it\'s'`,
	"workflow expression in string":      `v="a-${{ hashFiles('web/**', '}}') }}-b"`,
}

func TestShellReaderNeverSilentlyHidesACommand(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"escaped dollar before quote":      `echo \$'a\' ; ./pull.sh # '`,
		"PID parameter before quote":       `echo $$'a\' ; ./pull.sh # '`,
		"quote in quoted expansion":        "v=\"${x//'\"'/}\"\n./pull.sh\n# it's",
		"escaped backslash before newline": "echo a\\\\\n./pull.sh",
		"case after time":                  "v=\"$(time case x in x) echo '\"' ;; esac)\"\n./pull.sh\n# it's",
	}
	for name, command := range tests {
		t.Run(name, func(t *testing.T) {
			audit := &repositoryScriptAudit{
				sources: map[string]string{"pull.sh": "docker pull alpine\n"},
				state:   map[string]uint8{},
				result:  map[string]bool{},
			}
			if !audit.commandAcquires(command) {
				t.Fatalf("missed the command after %q", command)
			}
		})
	}
	// A comment's trailing backslash continues nothing.
	if !containerCommandInvokesEngine("true # note \\\nbash -c 'docker pull x'") {
		t.Error("a comment swallowed the line after it")
	}
}

func TestShellReaderSeesCommandAfterQuoting(t *testing.T) {
	t.Parallel()
	for name, line := range shellQuotingLines {
		t.Run(name, func(t *testing.T) {
			command := line + "\n./pull.sh now # note"
			segments := shellCommandSegments(command)
			if !slices.ContainsFunc(segments, func(segment []string) bool {
				return len(segment) >= 2 && segment[0] == "./pull.sh" && segment[1] == "now"
			}) {
				t.Errorf("segments lost the following command: %q", segments)
			}
			if tokens := shellCommandTokens(command); !slices.Contains(tokens, "./pull.sh") {
				t.Errorf("tokens lost the following command: %q", tokens)
			}
			if stripped := stripShellComments(command); !strings.HasSuffix(stripped, "\n./pull.sh now ") {
				t.Errorf("comment stripping lost its place: %q", stripped)
			}
			if err := checkShellSyntax(command); err != nil {
				t.Errorf("valid shell rejected: %v", err)
			}
		})
	}
}

func TestShellReaderReportsTheLineItCannotFollow(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"open double quote":           "true\nv=\"open\ntrue",
		"open single quote":           "true\nv='open",
		"open substitution":           "true\nv=$(open",
		"open quoted substitution":    "true\nv=\"$(true)$(\"",
		"open backtick":               "true\nv=`open",
		"unterminated heredoc":        "true\ncat <<EOF\nbody",
		"trailing escape":             "true\ntrue \\",
		"unbalanced arithmetic":       "true\nv=$((1)",
		"unterminated case in $(...)": "true\nv=$(case x in x) true )",
		"open parameter expansion":    "true\nv=${x",
		"PID parameter before quote":  "true\necho $$'a'",
		"quote in quoted expansion":   "true\nv=\"${x#'a'}\"",
		"continued heredoc delimiter": "true\ncat <<EOF\na \\\nEOF\nEOF",
		"continuation splits a token": "true\necho a\\\n#b",
	}
	for name, command := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkShellSyntax(command)
			if err == nil || !strings.HasPrefix(err.Error(), "line 2: ") {
				t.Fatalf("error = %v, want one naming line 2", err)
			}
			audit := &repositoryScriptAudit{sources: map[string]string{}, state: map[string]uint8{}, result: map[string]bool{}}
			if !audit.commandAcquires(command) {
				t.Fatal("script audit treated an unreadable command as not acquiring")
			}
		})
	}
}

func TestScriptAuditFollowsCommandsAfterQuotedSubstitution(t *testing.T) {
	t.Parallel()
	for name, line := range shellQuotingLines {
		if strings.Contains(line, "`") {
			continue // backticks are ambiguous to the substitution reader
		}
		for _, call := range []string{`./pull.sh`, `bash pull.sh`, `"$engine" pull x`} {
			audit := &repositoryScriptAudit{
				sources: map[string]string{"pull.sh": "docker pull alpine\n"},
				state:   map[string]uint8{},
				result:  map[string]bool{},
			}
			if !audit.commandAcquires(line + "\n" + call) {
				t.Errorf("%s: missed acquisition by %q", name, call)
			}
		}
	}
}

func TestVerifyCIContainerDownloadsSeesPastQuotedSubstitution(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		shellQuotingLines["double quote in single quotes"],
		shellQuotingLines["quoted sed pattern"],
	} {
		root := writeCIContainerDownloadsFixture(t)
		writeFixtureExecutable(t, filepath.Join(root, "scripts", "zz-pull.sh"), "#!/usr/bin/env bash\ndocker pull alpine\n")
		writeFixtureExecutable(t, filepath.Join(root, "scripts", "zz-helper.sh"), "#!/usr/bin/env bash\n"+line+"\n./scripts/zz-pull.sh\n")
		writeFixtureFile(t, filepath.Join(root, "mk", "zz.mk"), ".PHONY: android\nandroid:\n\t./scripts/zz-helper.sh\n")
		appendMakeInclude(t, root, "mk/zz.mk")
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Errorf("accepted acquisition behind %q", line)
		}
	}
}

func TestVerifyCIContainerDownloadsNamesUnreadableScript(t *testing.T) {
	t.Parallel()
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "zz-open.sh"), "#!/usr/bin/env bash\nv=\"open\n")
	err := VerifyCIContainerDownloads(root)
	if err == nil || !strings.Contains(err.Error(), "scripts/zz-open.sh: line 2: ") {
		t.Fatalf("error = %v, want one naming scripts/zz-open.sh line 2", err)
	}
}
