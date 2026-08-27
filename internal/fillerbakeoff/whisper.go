package fillerbakeoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxWhisperDiagnosticBytes = 64 << 10
	maxWhisperJSONBytes       = 2 << 20
)

type WhisperTranscriptConfig struct {
	BinaryPath            string
	BinarySHA256          string
	ImplementationVersion string
	ModelPath             string
	ModelSHA256           string
	TempDir               string
}

type whisperTranscriptEngine struct {
	binaryPath, modelPath, tempDir string
	identity                       TranscriptEngineIdentity
}

// NewWhisperTranscriptEngine pins the exact executable and model bytes before
// any corpus audio is written or subprocess is started.
func NewWhisperTranscriptEngine(config WhisperTranscriptConfig) (TranscriptEngine, error) {
	if strings.TrimSpace(config.ImplementationVersion) == "" || !validSHA256(config.BinarySHA256) || !validSHA256(config.ModelSHA256) {
		return nil, fmt.Errorf("whisper transcript engine requires version and exact binary/model digests")
	}
	if err := verifyRegularFileDigest(config.BinaryPath, config.BinarySHA256, true); err != nil {
		return nil, fmt.Errorf("whisper binary digest: %w", err)
	}
	if err := verifyRegularFileDigest(config.ModelPath, config.ModelSHA256, false); err != nil {
		return nil, fmt.Errorf("whisper model digest: %w", err)
	}
	return &whisperTranscriptEngine{
		binaryPath: config.BinaryPath, modelPath: config.ModelPath, tempDir: config.TempDir,
		identity: TranscriptEngineIdentity{
			Provider: "whisper.cpp", ImplementationVersion: config.ImplementationVersion,
			BinarySHA256: config.BinarySHA256, Model: filepath.Base(config.ModelPath), ModelSHA256: config.ModelSHA256,
		},
	}, nil
}

func (w *whisperTranscriptEngine) Identity() TranscriptEngineIdentity { return w.identity }

func (w *whisperTranscriptEngine) Transcribe(ctx context.Context, audio []byte) ([]TranscriptSegment, error) {
	if len(audio) == 0 || len(audio) > maxSignalBytes {
		return nil, fmt.Errorf("whisper audio is empty or exceeds the packet byte ceiling")
	}
	dir, err := os.MkdirTemp(w.tempDir, "loomarr-filler-transcript-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	wavPath := filepath.Join(dir, "input.wav")
	if err := os.WriteFile(wavPath, audio, 0o600); err != nil {
		return nil, fmt.Errorf("write whisper input: %w", err)
	}
	outputBase := filepath.Join(dir, "transcript")
	var diagnostic boundedBuffer
	diagnostic.limit = maxWhisperDiagnosticBytes
	command := exec.CommandContext(ctx, w.binaryPath, "-m", w.modelPath, "-f", wavPath, "-oj", "-of", outputBase, "-np")
	command.Stdout = &diagnostic
	command.Stderr = &diagnostic
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("whisper-cli: %w: %s", err, strings.TrimSpace(diagnostic.String()))
	}
	if diagnostic.exceeded {
		return nil, fmt.Errorf("whisper-cli diagnostic output exceeded %d bytes", maxWhisperDiagnosticBytes)
	}
	file, err := os.Open(outputBase + ".json")
	if err != nil {
		return nil, fmt.Errorf("whisper-cli wrote no JSON: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxWhisperJSONBytes+1))
	if err != nil || len(raw) > maxWhisperJSONBytes {
		return nil, fmt.Errorf("whisper JSON exceeded %d bytes", maxWhisperJSONBytes)
	}
	return decodeWhisperTranscript(raw)
}

type whisperTranscriptWire struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

func decodeWhisperTranscript(raw []byte) ([]TranscriptSegment, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var wire whisperTranscriptWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode whisper JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode whisper JSON: trailing value")
	}
	segments := make([]TranscriptSegment, 0, len(wire.Transcription))
	for _, segment := range wire.Transcription {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		segments = append(segments, TranscriptSegment{StartMS: segment.Offsets.From, EndMS: segment.Offsets.To, Text: text})
	}
	return segments, nil
}

func verifyRegularFileDigest(path, expected string, executable bool) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("%q is not a non-empty regular file", path)
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%q is not executable", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != expected {
		return fmt.Errorf("%q digest %s does not match %s", path, got, expected)
	}
	return nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.exceeded = true
		return original, nil
	}
	if len(data) > remaining {
		b.exceeded = true
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return original, nil
}

var _ TranscriptEngine = (*whisperTranscriptEngine)(nil)
