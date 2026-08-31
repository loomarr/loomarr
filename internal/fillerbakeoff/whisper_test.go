package fillerbakeoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhisperTranscriptEnginePinsFilesAndParsesTimedOutput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "whisper-fixture")
	script := `#!/bin/sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-of" ]; then out="$2"; shift 2; continue; fi
  shift
done
printf '%s' '{"transcription":[{"offsets":{"from":100,"to":1500},"text":" Buy now. "},{"offsets":{"from":1500,"to":4000},"text":"Call today."}]}' > "${out}.json"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "ggml-small.en.bin")
	if err := os.WriteFile(model, []byte("model fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	binaryHash := sha256.Sum256([]byte(script))
	modelHash := sha256.Sum256([]byte("model fixture"))
	engine, err := NewWhisperTranscriptEngine(WhisperTranscriptConfig{
		BinaryPath: binary, BinarySHA256: hex.EncodeToString(binaryHash[:]), ImplementationVersion: "v1.9.1",
		ModelPath: model, ModelSHA256: hex.EncodeToString(modelHash[:]), TempDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	segments, err := engine.Transcribe(context.Background(), []byte("wav bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 || segments[0].StartMS != 100 || segments[1].EndMS != 4_000 || segments[0].Text != "Buy now." {
		t.Fatalf("segments = %+v", segments)
	}
	identity := engine.Identity()
	if identity.Provider != "whisper.cpp" || identity.ImplementationVersion != "v1.9.1" || identity.BinarySHA256 != hex.EncodeToString(binaryHash[:]) || identity.Model != "ggml-small.en.bin" || identity.ModelSHA256 != hex.EncodeToString(modelHash[:]) {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestWhisperTranscriptEngineRejectsDigestDrift(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "whisper-fixture")
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewWhisperTranscriptEngine(WhisperTranscriptConfig{
		BinaryPath: binary, BinarySHA256: strings.Repeat("a", 64), ImplementationVersion: "v1.9.1",
		ModelPath: model, ModelSHA256: strings.Repeat("b", 64), TempDir: dir,
	})
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("digest drift error = %v", err)
	}
}
