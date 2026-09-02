// Command filler-reference-families fingerprints complete acquired sources and
// reports likely duplicate renditions. It never chooses or edits a rendition.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillerreference"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-reference-families", flag.ContinueOnError)
	flags.SetOutput(stderr)
	auditPath := flags.String("audit", "", "bound filler reference audit")
	sourceRoot := flags.String("source-root", "", "root containing acquired source files")
	ffmpegPath := flags.String("ffmpeg", "", "ffmpeg executable")
	outputPath := flags.String("output", "", "new immutable family audit JSON")
	generatedText := flags.String("generated-at", "", "fixed RFC3339 audit time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedText)
	if err != nil || *auditPath == "" || *sourceRoot == "" || *ffmpegPath == "" || *outputPath == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "filler-reference-families: audit, source-root, ffmpeg, output, and fixed generated-at are required")
		return 2
	}
	auditRaw, err := os.ReadFile(*auditPath)
	if err != nil {
		return fail(stderr, err)
	}
	audit, err := fillerbakeoffio.ReadStrictJSON[fillerreference.Audit](*auditPath)
	if err != nil {
		return fail(stderr, fmt.Errorf("audit: %w", err))
	}
	if audit.SchemaVersion != fillerreference.AuditSchemaVersion || audit.Contract != fillerreference.ContractVersion || len(audit.Cases) != audit.Summary.Cases {
		return fail(stderr, fmt.Errorf("audit identity or case count is invalid"))
	}
	fingerprints := make([]fillerreference.FamilyFingerprint, 0, len(audit.Cases))
	for _, item := range audit.Cases {
		if item.Disposition == fillerreference.DispositionExclude {
			continue
		}
		path, err := sourcePath(*sourceRoot, item.SourceLocalFile)
		if err != nil {
			return fail(stderr, fmt.Errorf("case %q: %w", item.CaseID, err))
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != item.ContentSHA256 {
			return fail(stderr, fmt.Errorf("case %q source identity mismatch: got %q: %w", item.CaseID, digest, err))
		}
		hashes, err := fingerprint(context.Background(), *ffmpegPath, path, item.Media.SourceDurationMS)
		if err != nil {
			return fail(stderr, fmt.Errorf("case %q: %w", item.CaseID, err))
		}
		audio, err := audioEnvelope(context.Background(), *ffmpegPath, path, item.Media.SourceDurationMS)
		if err != nil {
			return fail(stderr, fmt.Errorf("case %q: %w", item.CaseID, err))
		}
		fingerprints = append(fingerprints, fillerreference.FamilyFingerprint{
			CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, LocalFile: item.SourceLocalFile, FrameHashes: hashes, AudioRMS: audio,
		})
	}
	result, err := fillerreference.BuildFamilyAudit(auditRaw, fingerprints, generatedAt)
	if err != nil {
		return fail(stderr, err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	if err := publish(*outputPath, append(data, '\n')); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "filler-reference-families: %d cases, %d related pairs, %d families (%d non-clique)\n",
		result.Summary.Cases, result.Summary.RelatedPairs, result.Summary.DuplicateFamilies, result.Summary.NonCliqueFamilies)
	return 0
}

func sourcePath(root, local string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(local)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local file escapes source root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve source file: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local file escapes source root through symlink")
	}
	return resolvedPath, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fingerprint(ctx context.Context, ffmpegPath, path string, durationMS int64) ([]uint64, error) {
	if durationMS <= 0 {
		return nil, fmt.Errorf("source duration is not bounded")
	}
	var stdout, stderr bytes.Buffer
	filter := "fps=2,crop=trunc(iw*0.90/2)*2:trunc(ih*0.78/2)*2,scale=9:8,format=gray"
	cmd := exec.CommandContext(ctx, ffmpegPath, "-nostdin", "-hide_banner", "-loglevel", "error", "-i", path,
		"-t", strconv.FormatFloat(float64(durationMS)/1000, 'f', 3, 64), "-an", "-vf", filter, "-pix_fmt", "gray", "-f", "rawvideo", "-")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg fingerprint: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	const frameBytes = 9 * 8
	raw := stdout.Bytes()
	if len(raw)%frameBytes != 0 {
		return nil, fmt.Errorf("ffmpeg returned partial gray frame")
	}
	hashes := make([]uint64, 0, len(raw)/frameBytes)
	for len(raw) >= frameBytes {
		var hash uint64
		bit := uint(0)
		for row := 0; row < 8; row++ {
			for column := 0; column < 8; column++ {
				if raw[row*9+column] > raw[row*9+column+1] {
					hash |= 1 << bit
				}
				bit++
			}
		}
		hashes = append(hashes, hash)
		raw = raw[frameBytes:]
	}
	return hashes, nil
}

func audioEnvelope(ctx context.Context, ffmpegPath, path string, durationMS int64) ([]uint32, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpegPath, "-nostdin", "-hide_banner", "-loglevel", "error", "-i", path,
		"-t", strconv.FormatFloat(float64(durationMS)/1000, 'f', 3, 64), "-vn", "-ac", "1", "-ar", "8000", "-f", "s16le", "-")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg audio fingerprint: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	const samplesPerBin = 800
	raw := stdout.Bytes()
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("ffmpeg returned partial audio sample")
	}
	envelope := make([]uint32, 0, len(raw)/(samplesPerBin*2))
	for len(raw) >= samplesPerBin*2 {
		var squares int64
		for offset := 0; offset < samplesPerBin*2; offset += 2 {
			sample := int64(int16(binary.LittleEndian.Uint16(raw[offset : offset+2])))
			squares += sample * sample
		}
		envelope = append(envelope, uint32(math.Sqrt(float64(squares)/samplesPerBin)))
		raw = raw[samplesPerBin*2:]
	}
	return envelope, nil
}

func publish(path string, data []byte) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-reference-families-*")
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
	if err := temp.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable family audit: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "filler-reference-families:", err)
	return 1
}
