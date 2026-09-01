// Package fillerbakeoffio owns the strict file boundary shared by filler
// prediction capture commands.
package fillerbakeoffio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

func ReadStrictJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("trailing JSON value")
		}
		return value, err
	}
	return value, nil
}

func ReadPackets(path string) (map[string]fillerbakeoff.Packet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	packets := make(map[string]fillerbakeoff.Packet)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		packet, err := fillerbakeoff.DecodePacket(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if _, exists := packets[packet.CaseID]; exists {
			return nil, fmt.Errorf("line %d: duplicate case %q", line, packet.CaseID)
		}
		packets[packet.CaseID] = packet
	}
	return packets, scanner.Err()
}

func ReadTranscripts(path string) ([]fillerbakeoff.TranscriptArtifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var transcripts []fillerbakeoff.TranscriptArtifact
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var transcript fillerbakeoff.TranscriptArtifact
		if err := decoder.Decode(&transcript); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("line %d: trailing JSON value", line)
		}
		if _, duplicate := seen[transcript.CaseID]; duplicate {
			return nil, fmt.Errorf("line %d: duplicate case %q", line, transcript.CaseID)
		}
		seen[transcript.CaseID] = struct{}{}
		transcripts = append(transcripts, transcript)
	}
	return transcripts, scanner.Err()
}

func WritePredictions(path, tempPattern string, predictions []fillereval.Prediction) error {
	return writeImmutableJSONL(path, tempPattern, predictions)
}

func WriteTranscripts(path string, transcripts []fillerbakeoff.TranscriptArtifact) error {
	return writeImmutableJSONL(path, ".filler-bakeoff-transcripts-*", transcripts)
}

// WriteImmutableJSON publishes one indented JSON artifact without replacing an
// existing path. Evaluation evidence is append-only at the filesystem seam.
func WriteImmutableJSON(path, tempPattern string, value any) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), tempPattern)
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable JSON artifact: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}

func writeImmutableJSONL[T any](path, tempPattern string, values []T) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), tempPattern)
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return err
		}
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable JSONL ledger: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}
