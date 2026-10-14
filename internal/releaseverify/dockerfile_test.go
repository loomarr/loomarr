package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const boundedCurlPolicy = "--retry 5 --retry-all-errors --retry-delay 2 --retry-max-time 600"

var pinnedDownloadOutputs = []string{
	"/usr/local/bin/yt-dlp",
	"/tmp/deno.zip",
	"/tmp/ffmpeg.tar.xz",
	"/tmp/whisper.tar.gz",
	"/usr/local/share/whisper/ggml-small.en.bin",
	"/usr/local/share/whisper/ggml-tiny.bin",
}

func TestRepositoryDockerfileDownloadsUseBoundedRetries(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "..", "..", "Dockerfile")
	if err := VerifyDockerfileDownloads(path); err != nil {
		t.Fatalf("repository Dockerfile download policy: %v", err)
	}
}

func TestVerifyDockerfileDownloadsRequiresExactPolicyOnEveryActiveDownload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	complete := completeDownloadFixture()
	writeDockerfile(t, path, complete)
	if err := VerifyDockerfileDownloads(path); err != nil {
		t.Fatalf("complete download policy: %v", err)
	}

	mutations := []struct {
		name string
		old  string
		new  string
	}{
		{name: "missing retry count", old: "--retry 5 ", new: ""},
		{name: "wrong retry count", old: "--retry 5", new: "--retry 4"},
		{name: "missing all-errors", old: "--retry-all-errors ", new: ""},
		{name: "missing retry delay", old: "--retry-delay 2 ", new: ""},
		{name: "wrong retry delay", old: "--retry-delay 2", new: "--retry-delay 3"},
		{name: "missing retry window", old: "--retry-max-time 600 ", new: ""},
		{name: "wrong retry window", old: "--retry-max-time 600", new: "--retry-max-time 601"},
	}
	for _, output := range pinnedDownloadOutputs {
		output := output
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(filepath.Base(output)+"/"+mutation.name, func(t *testing.T) {
				command := "curl -fsSL " + boundedCurlPolicy + " -o " + output
				broken := strings.Replace(command, mutation.old, mutation.new, 1)
				if broken == command {
					t.Fatalf("test mutation %q did not change %q", mutation.name, command)
				}
				contents := strings.Replace(complete, command, broken, 1)
				if contents == complete {
					t.Fatalf("fixture has no command for %s", output)
				}
				writeDockerfile(t, path, contents)
				if err := VerifyDockerfileDownloads(path); err == nil {
					t.Fatalf("VerifyDockerfileDownloads accepted %s with %s", output, mutation.name)
				}
			})
		}
	}
}

func TestVerifyDockerfileDownloadsDoesNotTreatCommentsAsPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	contents := "FROM scratch\n# RUN curl -fsSL " + boundedCurlPolicy + " -o /tmp/commented https://example.invalid/commented\n"
	writeDockerfile(t, path, contents)
	if err := VerifyDockerfileDownloads(path); err == nil {
		t.Fatal("VerifyDockerfileDownloads accepted a policy present only in a comment")
	}

	contents += "RUN curl -fsSL -o /tmp/active https://example.invalid/active\n"
	writeDockerfile(t, path, contents)
	if err := VerifyDockerfileDownloads(path); err == nil {
		t.Fatal("VerifyDockerfileDownloads accepted an active download without retries because a comment had them")
	}
}

func TestVerifyDockerfileDownloadsDoesNotTreatInlineCommentsAsPolicy(t *testing.T) {
	t.Run("inline comment is not active policy", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		writeDockerfile(t, path, "FROM scratch\nRUN curl -fsSL -o /tmp/active https://example.invalid/active # "+boundedCurlPolicy+"\n")

		if err := VerifyDockerfileDownloads(path); err == nil {
			t.Fatal("VerifyDockerfileDownloads accepted retry policy text from an inline comment")
		}
	})

	t.Run("hash embedded in a token is not a comment", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		writeDockerfile(t, path, "FROM scratch\nRUN curl -fsSL "+boundedCurlPolicy+" -o /tmp/active https://example.invalid/active#fragment\n")

		if err := VerifyDockerfileDownloads(path); err != nil {
			t.Fatalf("VerifyDockerfileDownloads rejected a compliant download with a URL fragment: %v", err)
		}
	})
}

func completeDownloadFixture() string {
	var fixture strings.Builder
	fixture.WriteString("FROM scratch\n")
	fixture.WriteString("RUN apt-get install -y curl\n")
	for index, output := range pinnedDownloadOutputs {
		fixture.WriteString("RUN curl -fsSL ")
		fixture.WriteString(boundedCurlPolicy)
		fixture.WriteString(" -o ")
		fixture.WriteString(output)
		fixture.WriteString(" https://example.invalid/asset-")
		fixture.WriteString(string(rune('a' + index)))
		fixture.WriteByte('\n')
	}
	return fixture.String()
}

func writeDockerfile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
