package playoutcert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	operatorManifestLimit = 1 << 20
	operatorSourceLimit   = 256 << 20
	operatorTotalLimit    = 1 << 30
	operatorSourceCount   = 64
)

// OperatorCohort owns a contained source directory and immutable predeclared
// expectations. Loading or staging one does not qualify its media for playout.
// The caller closes it after staging; the staged copies have a separate owner.
type OperatorCohort struct {
	root           *os.Root
	manifestPath   string
	manifestSHA256 string
	entries        []operatorChannel
	sources        map[string]operatorSource
}

type operatorSource struct {
	file   string
	size   int64
	digest string
}

type operatorProgramme struct {
	source    operatorSource
	signature ProgrammeSignature
}

type operatorChannel struct {
	id         string
	programmes [2]operatorProgramme
}

// CohortMedia is an owned copy whose bytes matched the private input declaration.
// Its signature still requires actual observation through production playout.
type CohortMedia struct {
	Path      string
	Signature ProgrammeSignature
}

type StagedOperatorCohort struct {
	Directory string
	Channels  map[string][2]CohortMedia
}

type operatorManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Channels      []struct {
		ID         string `json:"id"`
		Programmes []struct {
			File    string `json:"file"`
			Bytes   int64  `json:"bytes"`
			SHA256  string `json:"sha256"`
			Signals struct {
				Luma             *operatorRange `json:"luma"`
				ZeroCrossingRate *operatorRange `json:"zeroCrossingRate"`
				RMSDB            *operatorRange `json:"rmsDB"`
				Silence          *bool          `json:"silence"`
			} `json:"signals"`
		} `json:"programmes"`
	} `json:"channels"`
}

type operatorRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

func (r *operatorRange) value() (SignalRange, bool) {
	if r == nil || r.Min == nil || r.Max == nil {
		return SignalRange{}, false
	}
	return SignalRange{Min: *r.Min, Max: *r.Max}, true
}

// LoadOperatorCohort validates the complete private declaration and every source
// without creating output, starting a process or requesting a network resource.
// All errors are fixed vocabulary: filesystem and JSON errors may contain private input.
func LoadOperatorCohort(ctx context.Context, manifestPath string, channels []Channel) (_ *OperatorCohort, resultErr error) {
	if ctx.Err() != nil || len(channels) < 1 || len(channels) > 1000 {
		return nil, errors.New("invalid operator cohort")
	}
	absolute, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, errors.New("invalid operator cohort path")
	}
	root, err := os.OpenRoot(filepath.Dir(absolute))
	if err != nil {
		return nil, errors.New("operator corpus unavailable")
	}
	defer func() {
		if resultErr != nil {
			_ = root.Close()
		}
	}()
	file, err := root.OpenFile(filepath.Base(absolute), operatorReadFlags, 0)
	if err != nil {
		return nil, errors.New("operator manifest unavailable")
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > operatorManifestLimit {
		_ = file.Close()
		return nil, errors.New("invalid operator manifest")
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, operatorManifestLimit+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(contents) > operatorManifestLimit || ctx.Err() != nil {
		return nil, errors.New("invalid operator manifest")
	}
	if err := uniqueOperatorJSON(contents); err != nil {
		return nil, errors.New("invalid operator manifest")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var manifest operatorManifest
	if decoder.Decode(&manifest) != nil || decoder.Decode(&struct{}{}) != io.EOF || manifest.SchemaVersion != 1 || len(manifest.Channels) != len(channels) {
		return nil, errors.New("invalid operator manifest")
	}
	manifestHash := sha256.Sum256(contents)
	cohort := &OperatorCohort{root: root, manifestPath: absolute, manifestSHA256: hex.EncodeToString(manifestHash[:]), sources: make(map[string]operatorSource)}
	seenChannels := make(map[string]bool)
	var total int64
	for index, declared := range manifest.Channels {
		if declared.ID == "" || declared.ID != channels[index].ID || seenChannels[declared.ID] || len(declared.Programmes) != 2 {
			return nil, errors.New("invalid operator channel sequence")
		}
		seenChannels[declared.ID] = true
		entry := operatorChannel{id: declared.ID}
		for variant, raw := range declared.Programmes {
			if !filepath.IsLocal(raw.File) || filepath.Clean(raw.File) != raw.File || raw.File == "." || raw.Bytes < 1 || raw.Bytes > operatorSourceLimit {
				return nil, errors.New("invalid operator source declaration")
			}
			digest, err := hex.DecodeString(raw.SHA256)
			if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != raw.SHA256 {
				return nil, errors.New("invalid operator source digest")
			}
			source := operatorSource{file: raw.File, size: raw.Bytes, digest: raw.SHA256}
			if previous, ok := cohort.sources[raw.File]; ok {
				if previous != source {
					return nil, errors.New("conflicting operator source declarations")
				}
			} else {
				total += raw.Bytes
				if len(cohort.sources) >= operatorSourceCount || total > operatorTotalLimit {
					return nil, errors.New("operator corpus exceeds bounds")
				}
				cohort.sources[raw.File] = source
			}
			luma, lv := raw.Signals.Luma.value()
			rate, rv := raw.Signals.ZeroCrossingRate.value()
			rms, mv := raw.Signals.RMSDB.value()
			if !lv || !rv || !mv || raw.Signals.Silence == nil {
				return nil, errors.New("incomplete operator signal declaration")
			}
			entry.programmes[variant] = operatorProgramme{source: source, signature: ProgrammeSignature{Luma: luma, ZeroCrossingRate: rate, RMSDB: rms, Silence: *raw.Signals.Silence}}
		}
		start := time.Unix(1, 0).UTC()
		middle := start.Add(time.Second)
		end := middle.Add(time.Second)
		expected := []ExpectedProgramme{entry.programmes[0].signature.Programme(start, middle), entry.programmes[1].signature.Programme(middle, end)}
		if validateProgrammeSequence(expected, start, end) != nil {
			return nil, errors.New("invalid operator signal sequence")
		}
		cohort.entries = append(cohort.entries, entry)
	}
	for _, source := range cohort.sources {
		if err := cohort.copySource(ctx, source, io.Discard); err != nil {
			return nil, err
		}
	}
	return cohort, nil
}

func (c *OperatorCohort) Close() error {
	if c == nil || c.root == nil {
		return nil
	}
	return c.root.Close()
}

// ManifestSHA256 identifies the exact admitted document, not its current path contents.
func (c *OperatorCohort) ManifestSHA256() string {
	if c == nil {
		return ""
	}
	return c.manifestSHA256
}

// MatchesChannels checks that a later target still uses the frozen ordered
// Channel mapping supplied to the loader.
func (c *OperatorCohort) MatchesChannels(channels []Channel) bool {
	if c == nil || len(channels) == 0 || len(c.entries) != len(channels) {
		return false
	}
	for index, channel := range channels {
		if c.entries[index].id != channel.ID {
			return false
		}
	}
	return true
}

// PrivateValues supplies the source strings which a publication audit must
// treat as private. The staged directory and paths must also be registered.
func (c *OperatorCohort) PrivateValues() []string {
	result := []string{c.manifestPath, filepath.Dir(c.manifestPath)}
	for _, source := range c.sources {
		result = append(result, source.file, filepath.Join(filepath.Dir(c.manifestPath), source.file), filepath.Base(source.file), source.digest)
	}
	return result
}

// Stage copies into a new caller-owned directory and checks the admitted bytes
// again. Failed staging removes every partial copy; original sources are read-only.
func (c *OperatorCohort) Stage(ctx context.Context, parent string) (_ StagedOperatorCohort, resultErr error) {
	if c == nil || c.root == nil || len(c.entries) == 0 || len(c.sources) == 0 {
		return StagedOperatorCohort{}, errors.New("invalid operator cohort")
	}
	if ctx.Err() != nil {
		return StagedOperatorCohort{}, errors.New("operator staging canceled")
	}
	directory, err := os.MkdirTemp(parent, "operator-sources-")
	if err != nil {
		return StagedOperatorCohort{}, errors.New("operator staging unavailable")
	}
	defer func() {
		if resultErr != nil {
			if os.RemoveAll(directory) != nil {
				resultErr = errors.New("operator staging cleanup failed")
			}
		}
	}()
	paths := make(map[operatorSource]string)
	for _, entry := range c.entries {
		for _, programme := range entry.programmes {
			if _, ok := paths[programme.source]; ok {
				continue
			}
			file, err := os.CreateTemp(directory, "source-*.media")
			if err != nil {
				return StagedOperatorCohort{}, errors.New("operator staging unavailable")
			}
			copyErr := c.copySource(ctx, programme.source, file)
			closeErr := file.Close()
			if copyErr != nil {
				return StagedOperatorCohort{}, copyErr
			}
			if closeErr != nil {
				return StagedOperatorCohort{}, errors.New("operator staging close failed")
			}
			paths[programme.source] = file.Name()
		}
	}
	staged := StagedOperatorCohort{Directory: directory, Channels: make(map[string][2]CohortMedia)}
	for _, entry := range c.entries {
		var pair [2]CohortMedia
		for i, programme := range entry.programmes {
			pair[i] = CohortMedia{Path: paths[programme.source], Signature: programme.signature}
		}
		staged.Channels[entry.id] = pair
	}
	return staged, nil
}

func (c *OperatorCohort) copySource(ctx context.Context, source operatorSource, output io.Writer) error {
	file, err := c.root.OpenFile(source.file, operatorReadFlags, 0)
	if err != nil {
		return errors.New("operator source unavailable")
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.size {
		return errors.New("operator source size mismatch")
	}
	hash := sha256.New()
	read := &operatorSourceReader{ctx: ctx, reader: io.LimitReader(file, source.size+1)}
	count, err := io.Copy(io.MultiWriter(output, hash), read)
	if err != nil || ctx.Err() != nil {
		return errors.New("operator source read failed")
	}
	if count != source.size || hex.EncodeToString(hash.Sum(nil)) != source.digest {
		return errors.New("operator source digest mismatch")
	}
	closeErr := file.Close()
	file = nil
	if closeErr != nil {
		return errors.New("operator source close failed")
	}
	return nil
}

type operatorSourceReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *operatorSourceReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Check duplicate keys before decoding into structs, where encoding/json would
// silently keep the last value. Depth and input size bound the token walk.
func uniqueOperatorJSON(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 12 {
			return errors.New("operator JSON depth exceeded")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			keys := make(map[string]bool)
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok {
					return errors.New("invalid operator JSON key")
				}
				key = strings.ToLower(key)
				if keys[key] {
					return errors.New("duplicate operator JSON key")
				}
				keys[key] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid operator JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing operator JSON")
	}
	return nil
}
