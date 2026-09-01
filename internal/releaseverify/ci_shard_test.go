package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoShardUsesSerpentineRows(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	const packages = `example.invalid/a
example.invalid/b
example.invalid/c
example.invalid/d
example.invalid/e
example.invalid/f
example.invalid/g
example.invalid/h
example.invalid/i`
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	want := map[string]string{
		"1/3": "example.invalid/a\nexample.invalid/f\nexample.invalid/g",
		"2/3": "example.invalid/b\nexample.invalid/e\nexample.invalid/h",
		"3/3": "example.invalid/c\nexample.invalid/d\nexample.invalid/i",
	}
	for shard, expected := range want {
		shard, expected := shard, expected
		t.Run(shard, func(t *testing.T) {
			cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), shard)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go shard %s: %v\n%s", shard, err, output)
			}
			if got := strings.TrimSpace(string(output)); got != expected {
				t.Fatalf("go shard %s =\n%s\nwant serpentine row assignment\n%s", shard, got, expected)
			}
		})
	}
}
