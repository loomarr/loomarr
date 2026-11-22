package rustgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The decode-subtitles ceilings of design images.md "Operations beyond renditions" that a result
// shows. The worker enforces them, and its track-wide work ceilings, while decoding; the adapter
// re-checks every one of these against the result.
const (
	MaxSubtitleInputBytes  = 256 << 20
	MaxSubtitleWidth       = 3840
	MaxSubtitleHeight      = 2160
	MaxSubtitleStates      = 50_000
	MaxSubtitleOutputBytes = 512 << 20

	// CodecPGS is the Blu-ray image subtitle codec, an extracted `.sup` track.
	CodecPGS = "hdmv_pgs_subtitle"

	decodeSubtitlesOperation = "decode-subtitles"
	// A result lists up to 50,000 states of about 150 bytes each.
	maxSubtitleResultBytes = 16 << 20
)

// SubtitleRequest names one extracted image subtitle track and the private staging directory the
// worker writes its stills into.
type SubtitleRequest struct {
	RequestID      string
	Codec          string
	Path           string
	ExpectedSHA256 string
	StagingDir     string
}

// SubtitleStills is a decoded track: the canvas its compositions declare and its display states
// in time order. Files remain unpublished in the request's StagingDir.
type SubtitleStills struct {
	Width, Height int
	States        []SubtitleState
}

// SubtitleState is what shows from StartMS until EndMS, the next state's start. The last state has
// no successor and holds until the programme ends, so its EndMS equals its StartMS. File is a
// staging-relative full-canvas straight-alpha PNG; "" means the screen is clear.
type SubtitleState struct {
	StartMS, EndMS int64
	File           string
}

type subtitleRequest struct {
	Protocol   int            `json:"protocol"`
	RequestID  string         `json:"requestId"`
	Source     subtitleSource `json:"source"`
	StagingDir string         `json:"stagingDir"`
}

type subtitleSource struct {
	Codec          string `json:"codec"`
	Path           string `json:"path"`
	ExpectedSHA256 string `json:"expectedSha256"`
}

type subtitleManifest struct {
	Protocol  int    `json:"protocol"`
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
	Source    struct {
		Codec  string `json:"codec"`
		SHA256 string `json:"sha256"`
		Bytes  int64  `json:"bytes"`
	} `json:"source"`
	Width  int `json:"width"`
	Height int `json:"height"`
	States []struct {
		StartMS int64 `json:"startMs"`
		EndMS   int64 `json:"endMs"`
		Still   *struct {
			RelativePath string `json:"relativePath"`
			Bytes        int64  `json:"bytes"`
			SHA256       string `json:"sha256"`
		} `json:"still"`
	} `json:"states"`
}

// DecodeSubtitles runs exactly one bounded worker process and accepts only a result whose every
// still is a complete, declared 8-bit RGBA PNG of the canvas size inside StagingDir. On any error
// the staging directory can hold partial or refused stills; the caller removes it.
func (g *Generator) DecodeSubtitles(ctx context.Context, req SubtitleRequest) (SubtitleStills, error) {
	if !slices.Contains(g.operations, decodeSubtitlesOperation) {
		return SubtitleStills{}, fmt.Errorf("images: worker lacks the %s operation", decodeSubtitlesOperation)
	}
	payload, err := json.Marshal(subtitleRequest{
		Protocol: g.contract.Protocol, RequestID: req.RequestID, StagingDir: req.StagingDir,
		Source: subtitleSource{Codec: req.Codec, Path: req.Path, ExpectedSHA256: req.ExpectedSHA256},
	})
	if err != nil {
		return SubtitleStills{}, fmt.Errorf("images: worker request JSON: %w", err)
	}
	cmd := exec.CommandContext(ctx, g.executable, decodeSubtitlesOperation, "--protocol", strconv.Itoa(g.contract.Protocol))
	cmd.Stdin = bytes.NewReader(payload)
	stdout := newLimitBuffer(maxSubtitleResultBytes)
	stderr := newLimitBuffer(64 << 10)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if stdout.exceeded || stderr.exceeded {
		return SubtitleStills{}, fmt.Errorf("images: worker output exceeded protocol limit")
	}
	if runErr != nil {
		var failure errorManifest
		if json.Unmarshal(stdout.Bytes(), &failure) == nil && failure.Protocol == g.contract.Protocol &&
			failure.Status == "error" && failure.RequestID == req.RequestID && failure.Error.Code != "" {
			return SubtitleStills{}, &WorkerError{Code: failure.Error.Code, Message: failure.Error.Message}
		}
		return SubtitleStills{}, fmt.Errorf("images: worker decode-subtitles: %w: %s", runErr, strings.TrimSpace(stderr.String()))
	}
	var manifest subtitleManifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		return SubtitleStills{}, fmt.Errorf("images: worker result JSON: %w", err)
	}
	return g.verifySubtitles(req, manifest)
}

func (g *Generator) verifySubtitles(req SubtitleRequest, m subtitleManifest) (SubtitleStills, error) {
	if m.Protocol != g.contract.Protocol || m.Status != "ok" || m.RequestID != req.RequestID {
		return SubtitleStills{}, fmt.Errorf("images: worker result envelope mismatch")
	}
	if m.Source.Codec != req.Codec || m.Source.SHA256 != req.ExpectedSHA256 ||
		m.Source.Bytes <= 0 || m.Source.Bytes > MaxSubtitleInputBytes {
		return SubtitleStills{}, fmt.Errorf("images: worker source does not match the request")
	}
	if err := verifySource(req.Path, req.ExpectedSHA256, m.Source.Bytes); err != nil {
		return SubtitleStills{}, err
	}
	if m.Width <= 0 || m.Height <= 0 || m.Width > MaxSubtitleWidth || m.Height > MaxSubtitleHeight ||
		len(m.States) > MaxSubtitleStates {
		return SubtitleStills{}, fmt.Errorf("images: worker returned an out-of-ceiling subtitle track")
	}
	out := SubtitleStills{Width: m.Width, Height: m.Height, States: make([]SubtitleState, 0, len(m.States))}
	seen := make(map[string]bool, len(m.States))
	var total int64
	for i, state := range m.States {
		last := i == len(m.States)-1
		if state.StartMS < 0 || (i > 0 && state.StartMS <= m.States[i-1].StartMS) ||
			(!last && state.EndMS != m.States[i+1].StartMS) || (last && state.EndMS != state.StartMS) {
			return SubtitleStills{}, fmt.Errorf("images: worker state %d has an invalid timeline", i)
		}
		if state.Still == nil {
			// The worker folds repeated clears, so a clear always follows a still.
			if i == 0 || m.States[i-1].Still == nil {
				return SubtitleStills{}, fmt.Errorf("images: worker state %d is an unfolded clear", i)
			}
			out.States = append(out.States, SubtitleState{StartMS: state.StartMS, EndMS: state.EndMS})
			continue
		}
		name := state.Still.RelativePath
		if name == "" || filepath.Base(name) != name || filepath.Clean(name) != name || seen[name] {
			return SubtitleStills{}, fmt.Errorf("images: worker returned unsafe still path %q", name)
		}
		seen[name] = true
		total += state.Still.Bytes
		if state.Still.Bytes <= 0 || total > MaxSubtitleOutputBytes {
			return SubtitleStills{}, fmt.Errorf("images: worker stills exceed the output ceiling")
		}
		if err := verifyStill(filepath.Join(req.StagingDir, name), state.Still.Bytes, state.Still.SHA256, m.Width, m.Height); err != nil {
			return SubtitleStills{}, err
		}
		out.States = append(out.States, SubtitleState{StartMS: state.StartMS, EndMS: state.EndMS, File: name})
	}
	return out, nil
}

// verifySource re-reads the track so a file that changed during decoding is never accepted.
func verifySource(path, expected string, size int64) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return fmt.Errorf("images: worker source does not match the declared regular file")
	}
	file, err := os.Open(path) //nolint:gosec // caller supplies a private extracted track path
	if err != nil {
		return fmt.Errorf("images: open worker source: %w", err)
	}
	hash := sha256.New()
	n, readErr := io.Copy(hash, io.LimitReader(file, size+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || n != size || fmt.Sprintf("%x", hash.Sum(nil)) != expected {
		return fmt.Errorf("images: worker source bytes changed during decoding")
	}
	return nil
}

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// The IHDR bit depth and colour type of the worker's straight-alpha stills.
const (
	pngBitDepth8  = 8
	pngColourRGBA = 6
)

// verifyStill accepts only the declared regular file: its size, hash, and a PNG header whose
// IHDR declares the canvas size, bit depth 8 and colour type 6 (RGBA).
func verifyStill(path string, size int64, digest string, width, height int) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return fmt.Errorf("images: worker still %q is not the declared regular file", filepath.Base(path))
	}
	file, err := os.Open(path) //nolint:gosec // the path is a verified base name inside staging
	if err != nil {
		return fmt.Errorf("images: open worker still: %w", err)
	}
	header := make([]byte, 26)
	_, headerErr := io.ReadFull(file, header)
	hash := sha256.New()
	_, _ = hash.Write(header)
	_, readErr := io.Copy(hash, file)
	closeErr := file.Close()
	if headerErr != nil || readErr != nil || closeErr != nil {
		return fmt.Errorf("images: read worker still %q", filepath.Base(path))
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != digest {
		return fmt.Errorf("images: worker still %q hash mismatch", filepath.Base(path))
	}
	if !bytes.Equal(header[:8], pngSignature) || string(header[12:16]) != "IHDR" ||
		binary.BigEndian.Uint32(header[16:20]) != uint32(width) || binary.BigEndian.Uint32(header[20:24]) != uint32(height) || //nolint:gosec // width and height are within the 3840x2160 ceiling
		header[24] != pngBitDepth8 || header[25] != pngColourRGBA {
		return fmt.Errorf("images: worker still %q is not a canvas-sized RGBA PNG", filepath.Base(path))
	}
	return nil
}
