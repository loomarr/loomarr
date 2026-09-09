package releaseverify

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIFFmpegPinnedArchive(t *testing.T) {
	for _, scenario := range []string{"cached", "download", "corrupt cache", "corrupt download", "wrong version", "missing probe"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			_, source, _, _ := runtime.Caller(0)
			script := filepath.Join(root, "scripts", "ci-ffmpeg.sh")
			writeFixtureExecutable(t, script, readFixtureFile(t, filepath.Join(filepath.Dir(source), "..", "..", "scripts", "ci-ffmpeg.sh")))
			const buildID = "n8.1.2-34-g9b6c8969e0"
			const build = "ffmpeg-" + buildID + "-linux64-gpl-8.1"
			bin := filepath.Join(root, "commands")
			writeFixtureExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf 'x86_64\\n'\n")
			writeFixtureExecutable(t, filepath.Join(bin, "curl"), `#!/bin/sh
set -eu
printf 'download\n' >> "$FETCH_LOG"
while [ "$#" -gt 0 ]; do
 if [ "$1" = --output ]; then shift; cp "$FIXTURE_ARCHIVE" "$1"; exit; fi
 shift
done
exit 2
`)
			for _, tool := range []string{"ffmpeg", "ffprobe"} {
				if scenario == "missing probe" && tool == "ffprobe" {
					continue
				}
				version := buildID + "-20260731"
				if scenario == "wrong version" && tool == "ffprobe" {
					version = "n6.1.1"
				}
				writeFixtureExecutable(t, filepath.Join(root, build, "bin", tool), "#!/bin/sh\nprintf '"+tool+" version "+version+" fixture\\n'\n")
			}
			archive := filepath.Join(root, "fixture.tar.xz")
			cmd := exec.Command("tar", "-cJf", archive, "-C", root, build)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fixture archive: %v\n%s", err, out)
			}
			bytes, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(bytes))
			writeFixtureFile(t, filepath.Join(root, "Dockerfile"), "ARG FFMPEG_RELEASE=autobuild-2026-07-31-14-10\nARG FFMPEG_BUILD_ID="+buildID+"\nARG FFMPEG_AMD64_SHA256="+digest+"\n")
			cache := filepath.Join(root, "cache")
			cached := filepath.Join(cache, build+".tar.xz")
			if scenario != "download" && scenario != "corrupt download" {
				writeFixtureFile(t, cached, string(bytes))
			}
			if scenario == "corrupt cache" {
				writeFixtureFile(t, cached, "untrusted")
			}
			if scenario == "corrupt download" {
				writeFixtureFile(t, archive, "untrusted")
			}
			run := func(args ...string) (string, error) {
				t.Helper()
				cmd := exec.Command("/bin/bash", append([]string{script}, args...)...)
				cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FETCH_LOG="+filepath.Join(root, "fetch.log"), "FIXTURE_ARCHIVE="+archive)
				out, err := cmd.CombinedOutput()
				return string(out), err
			}
			if out, err := run("metadata"); err != nil || out != "sha256="+digest+"\n" {
				t.Fatalf("metadata: %q, %v", out, err)
			}
			out, err := run("download", cache)
			corrupt := strings.HasPrefix(scenario, "corrupt")
			if corrupt {
				if err == nil {
					t.Fatal("untrusted archive accepted")
				}
				if _, err := os.Stat(filepath.Join(root, "installed")); !os.IsNotExist(err) {
					t.Fatal("download extracted untrusted content")
				}
				return
			}
			if err != nil {
				t.Fatalf("download: %v\n%s", err, out)
			}
			fetches, err := os.ReadFile(filepath.Join(root, "fetch.log"))
			if scenario == "download" {
				if err != nil || string(fetches) != "download\n" {
					t.Fatalf("fetches %q: %v", fetches, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("cache hit accessed network: %q, %v", fetches, err)
			}
			out, err = run("install", cache, filepath.Join(root, "installed"))
			invalid := scenario == "wrong version" || scenario == "missing probe"
			if invalid == (err == nil) {
				t.Fatalf("install for %s: %v\n%s", scenario, err, out)
			}
			if !invalid {
				// Installation must independently verify the cache, even after a successful download.
				writeFixtureFile(t, cached, "changed after download")
				if out, err := run("install", cache, filepath.Join(root, "rejected")); err == nil {
					t.Fatalf("changed cache accepted: %s", out)
				}
				if _, err := os.Stat(filepath.Join(root, "rejected")); !os.IsNotExist(err) {
					t.Fatal("corrupt install extracted content")
				}
			}
		})
	}
}

func TestCIFFmpegArchitecturePins(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(source), "..", "..")
	pins := readFixtureFile(t, filepath.Join(repo, "Dockerfile"))
	scriptSource := readFixtureFile(t, filepath.Join(repo, "scripts", "ci-ffmpeg.sh"))
	for _, arch := range []string{"x86_64", "aarch64", "arm64", "unsupported"} {
		t.Run(arch, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "scripts", "ci-ffmpeg.sh")
			writeFixtureExecutable(t, script, scriptSource)
			writeFixtureFile(t, filepath.Join(root, "Dockerfile"), pins)
			bin := filepath.Join(root, "bin")
			writeFixtureExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '"+arch+"\\n'\n")
			cmd := exec.Command("/bin/bash", script, "metadata")
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if arch == "unsupported" {
				if err == nil {
					t.Fatal("unsupported architecture accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("metadata: %v\n%s", err, out)
			}
			pin := "FFMPEG_ARM64_SHA256"
			if arch == "x86_64" {
				pin = "FFMPEG_AMD64_SHA256"
			}
			var want string
			for line := range strings.SplitSeq(pins, "\n") {
				if value, ok := strings.CutPrefix(line, "ARG "+pin+"="); ok {
					want = "sha256=" + value + "\n"
				}
			}
			if want == "" || string(out) != want {
				t.Fatalf("architecture pin %q, want %q", out, want)
			}
			writeFixtureFile(t, filepath.Join(root, "Dockerfile"), pins+"\nARG "+pin+"="+strings.Repeat("0", 64)+"\n")
			cmd = exec.Command("/bin/bash", script, "metadata")
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("duplicate pin accepted: %s", out)
			}
		})
	}
}

func TestCIFFmpegWorkflowUsesVerifiedPair(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "ci-go.yml")), &document); err != nil {
		t.Fatal(err)
	}
	steps := mappingValueMust(t, workflowJobNode(t, document.Content[0], "run"), "steps")
	commands := map[int]string{
		5: `./scripts/ci-ffmpeg.sh metadata >> "$GITHUB_OUTPUT"`,
		7: `./scripts/ci-ffmpeg.sh download "$HOME/.cache/loomarr-ffmpeg"`,
		8: "./scripts/ci-ffmpeg.sh install \"$HOME/.cache/loomarr-ffmpeg\" \"$RUNNER_TEMP/loomarr-ffmpeg\"\necho \"$RUNNER_TEMP/loomarr-ffmpeg\" >> \"$GITHUB_PATH\"",
		9: "test \"$(command -v ffmpeg)\" = \"$RUNNER_TEMP/loomarr-ffmpeg/ffmpeg\"\ntest \"$(command -v ffprobe)\" = \"$RUNNER_TEMP/loomarr-ffmpeg/ffprobe\"\nffmpeg -version\nffprobe -version",
	}
	for index, want := range commands {
		if index >= len(steps.Content) {
			t.Fatalf("missing FFmpeg step %d", index)
		}
		step := steps.Content[index]
		run, ok := mappingValue(step, "run")
		if !ok || strings.TrimSpace(run.Value) != want {
			t.Fatalf("FFmpeg step %d no longer uses verified production pair", index)
		}
		for _, key := range []string{"if", "continue-on-error", "env", "shell", "working-directory"} {
			if _, ok := mappingValue(step, key); ok {
				t.Fatalf("FFmpeg step %d overrides %s", index, key)
			}
		}
	}
}
