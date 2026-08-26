// Command filler-corpus-prepare turns rights-approved media into a provenance-
// complete unlabeled draft and label-blind evidence packets.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/mediatools"
)

const preparationSchemaVersion = 1

type preparationPlan struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	CorpusVersion   string                 `json:"corpusVersion"`
	EvidenceVersion string                 `json:"evidenceVersion"`
	SliceGates      []fillereval.SliceGate `json:"sliceGates"`
	Cases           []plannedCase          `json:"cases"`
}

type plannedCase struct {
	CaseID            string           `json:"caseId"`
	Split             fillereval.Split `json:"split"`
	Cluster           string           `json:"cluster"`
	SegmentStartMS    int64            `json:"segmentStartMs"`
	SegmentDurationMS int64            `json:"segmentDurationMs"`
	VideoStartMS      int64            `json:"videoStartMs"`
	VideoDurationMS   int64            `json:"videoDurationMs"`
}

type mediaMeasurement struct {
	DurationMS int64
	Usable     bool
	Detail     string
}

type videoDerivative struct {
	Data          []byte
	SHA256        string
	DurationMS    int64
	Width, Height int
}

type mediaDeriver interface {
	Measure(context.Context, string, int64, int64) (mediaMeasurement, error)
	Frames(context.Context, string, int64, int64) ([][]byte, error)
	Video(context.Context, string, int64, int64) (videoDerivative, error)
}

type realDeriver struct {
	ffmpeg string
	tools  *mediatools.FFmpegTools
}

type options struct {
	inventoryPath, approvalsPath, planPath, localRoot, remoteRoot string
	draftOut, packetsOut, derivativesRoot                         string
	preparedAt                                                    time.Time
	minItems, maxItems                                            int
	maxInputBytes, maxOutputBytes                                 int64
	maxWallTime                                                   time.Duration
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "strict mixed-authority inventory JSON")
	approvalsPath := flags.String("rights-approvals", "", "locked rights decisions JSONL")
	planPath := flags.String("plan", "", "authored split, cluster, and segment plan JSON")
	localRoot := flags.String("local-root", "", "direct-cohort media root")
	remoteRoot := flags.String("remote-root", "", "downloaded public media root")
	draftOut := flags.String("draft-out", "", "unlabeled certification draft JSON")
	packetsOut := flags.String("packets-out", "", "label-blind packet JSONL")
	derivativesRoot := flags.String("derivatives-root", "", "external bounded derivative root")
	preparedText := flags.String("prepared-at", "", "fixed RFC3339 preparation time")
	ffmpegPath := flags.String("ffmpeg", "ffmpeg", "ffmpeg executable")
	minItems := flags.Int("min-items", 300, "minimum complete corpus cases")
	maxItems := flags.Int("max-items", 500, "maximum complete corpus cases")
	maxInputBytes := flags.Int64("max-input-bytes", 0, "aggregate source-media byte ceiling")
	maxOutputBytes := flags.Int64("max-output-bytes", 0, "aggregate derivative byte ceiling")
	maxWall := flags.Duration("max-wall-time", 0, "complete preparation wall-time ceiling")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedText)
	if err != nil || *inventoryPath == "" || *approvalsPath == "" || *planPath == "" || *localRoot == "" || *remoteRoot == "" || *draftOut == "" || *packetsOut == "" || *derivativesRoot == "" || *minItems <= 0 || *maxItems < *minItems || *maxInputBytes <= 0 || *maxOutputBytes <= 0 || *maxWall <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-prepare: all paths, valid preparation time, item bounds, byte ceilings, and wall-time ceiling are required")
		return 2
	}
	opts := options{inventoryPath: *inventoryPath, approvalsPath: *approvalsPath, planPath: *planPath, localRoot: *localRoot, remoteRoot: *remoteRoot, draftOut: *draftOut, packetsOut: *packetsOut, derivativesRoot: *derivativesRoot, preparedAt: preparedAt.UTC(), minItems: *minItems, maxItems: *maxItems, maxInputBytes: *maxInputBytes, maxOutputBytes: *maxOutputBytes, maxWallTime: *maxWall}
	deriver := &realDeriver{ffmpeg: *ffmpegPath, tools: mediatools.NewFFmpegTools(*ffmpegPath, filler.FFprobePathNextTo(*ffmpegPath), "", "", "")}
	draft, packets, err := prepare(context.Background(), opts, deriver)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-prepare:", err)
		return 1
	}
	if err := writeJSON(opts.draftOut, draft); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-prepare: write draft:", err)
		return 1
	}
	if err := writeJSONL(opts.packetsOut, packets); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-prepare: write packets:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-prepare: froze %d draft cases and evidence packets\n", len(draft.Cases))
	return 0
}

func prepare(ctx context.Context, opts options, deriver mediaDeriver) (fillereval.Manifest, []fillerbakeoff.Packet, error) {
	started := time.Now()
	inventoryRaw, err := os.ReadFile(opts.inventoryPath)
	if err != nil {
		return fillereval.Manifest{}, nil, err
	}
	inv, err := fillercorpus.DecodeInventoryBytes(inventoryRaw)
	if err != nil {
		return fillereval.Manifest{}, nil, err
	}
	inventoryDigest := fillercorpus.InventorySHA256(inventoryRaw)
	approvals, err := readJSONL[fillercorpus.RightsDecision](opts.approvalsPath)
	if err != nil {
		return fillereval.Manifest{}, nil, err
	}
	var plan preparationPlan
	if err := readStrictJSON(opts.planPath, &plan); err != nil {
		return fillereval.Manifest{}, nil, err
	}
	if plan.SchemaVersion != preparationSchemaVersion || strings.TrimSpace(plan.CorpusVersion) == "" || strings.TrimSpace(plan.EvidenceVersion) == "" || len(plan.SliceGates) == 0 || len(plan.Cases) < opts.minItems || len(plan.Cases) > opts.maxItems || len(plan.Cases) != len(inv.Cases) {
		return fillereval.Manifest{}, nil, fmt.Errorf("plan identity, gates, or complete item count is invalid")
	}
	if err := validateSliceGates(plan.SliceGates, len(plan.Cases)); err != nil {
		return fillereval.Manifest{}, nil, err
	}
	if _, err := os.Stat(opts.derivativesRoot); !os.IsNotExist(err) {
		return fillereval.Manifest{}, nil, fmt.Errorf("derivative output already exists")
	}
	if err := os.MkdirAll(filepath.Dir(opts.derivativesRoot), 0o750); err != nil {
		return fillereval.Manifest{}, nil, err
	}
	stageRoot, err := os.MkdirTemp(filepath.Dir(opts.derivativesRoot), ".filler-corpus-derivatives-*")
	if err != nil {
		return fillereval.Manifest{}, nil, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stageRoot)
		}
	}()

	byCase := make(map[string]fillercorpus.InventoryCase, len(inv.Cases))
	for _, item := range inv.Cases {
		byCase[item.CaseID] = item
	}
	approved := make(map[string]fillercorpus.RightsDecision, len(approvals))
	for _, decision := range approvals {
		if _, duplicate := approved[decision.CaseID]; duplicate {
			return fillereval.Manifest{}, nil, fmt.Errorf("duplicate rights decision %q", decision.CaseID)
		}
		approved[decision.CaseID] = decision
	}
	seenCases := map[string]struct{}{}
	clusterSplits := map[string]fillereval.Split{}
	contentCases := map[string]string{}
	splitCounts := map[fillereval.Split]int{}
	draft := fillereval.Manifest{SchemaVersion: fillereval.SchemaVersion, Kind: fillereval.CorpusCertification, CorpusVersion: plan.CorpusVersion, SliceGates: slices.Clone(plan.SliceGates)}
	packets := make([]fillerbakeoff.Packet, 0, len(plan.Cases))
	var inputBytes, outputBytes int64
	for _, planned := range plan.Cases {
		if time.Since(started) > opts.maxWallTime {
			return fillereval.Manifest{}, nil, fmt.Errorf("wall-time ceiling exceeded")
		}
		item, ok := byCase[planned.CaseID]
		if !ok {
			return fillereval.Manifest{}, nil, fmt.Errorf("plan case %q is absent from inventory", planned.CaseID)
		}
		if _, duplicate := seenCases[planned.CaseID]; duplicate {
			return fillereval.Manifest{}, nil, fmt.Errorf("duplicate plan case %q", planned.CaseID)
		}
		seenCases[planned.CaseID] = struct{}{}
		if planned.Split != fillereval.SplitDevelopment && planned.Split != fillereval.SplitHoldout || strings.TrimSpace(planned.Cluster) == "" || planned.SegmentStartMS < 0 || planned.SegmentDurationMS <= 0 || planned.VideoStartMS < planned.SegmentStartMS || planned.VideoDurationMS <= 0 || planned.VideoDurationMS > mediatools.HostedVideoMaxDurationMS || planned.VideoStartMS+planned.VideoDurationMS > planned.SegmentStartMS+planned.SegmentDurationMS {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q has invalid split, cluster, or bounded spans", planned.CaseID)
		}
		if prior, exists := clusterSplits[planned.Cluster]; exists && prior != planned.Split {
			return fillereval.Manifest{}, nil, fmt.Errorf("cluster %q crosses splits", planned.Cluster)
		}
		clusterSplits[planned.Cluster] = planned.Split
		splitCounts[planned.Split]++
		approval, ok := approved[planned.CaseID]
		if !ok || approval.InventorySHA256 != inventoryDigest || approval.Decision != "approved" || !approval.Redistributable || approval.MetadataSHA256 != item.MetadataSHA256 || approval.CaptureID != item.CaptureID || approval.Authority != item.Authority || approval.ItemID != item.ItemID || approval.ReviewerID == "" || strings.TrimSpace(approval.Basis) == "" || approval.ReviewedAt.Before(item.MetadataRetrievedAt) || approval.ReviewedAt.After(opts.preparedAt) {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q lacks a complete redistribution approval bound to this inventory", planned.CaseID)
		}
		mediaPath, sourceRef, err := mediaPathFor(opts, item)
		if err != nil {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q: %w", planned.CaseID, err)
		}
		hashes, size, err := hashMedia(mediaPath)
		if err != nil {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q: %w", planned.CaseID, err)
		}
		if size != item.Representation.Bytes || !matches(item.Representation.SHA256, hashes.sha256) || !matches(item.Representation.SHA1, hashes.sha1) || !matches(item.Representation.MD5, hashes.md5) {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q media identity differs from inventory", planned.CaseID)
		}
		if prior := contentCases[hashes.sha256]; prior != "" {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q duplicates media bytes from %q", planned.CaseID, prior)
		}
		contentCases[hashes.sha256] = planned.CaseID
		inputBytes += size
		if inputBytes > opts.maxInputBytes {
			return fillereval.Manifest{}, nil, fmt.Errorf("source media exceeds aggregate byte ceiling")
		}
		measurement, err := deriver.Measure(ctx, mediaPath, planned.SegmentStartMS, planned.SegmentStartMS+planned.SegmentDurationMS)
		if err != nil || measurement.DurationMS < planned.SegmentStartMS+planned.SegmentDurationMS {
			return fillereval.Manifest{}, nil, fmt.Errorf("case %q media measurement is incomplete: %w", planned.CaseID, err)
		}
		packet := fillerbakeoff.Packet{SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: planned.CaseID, EvidenceVersion: plan.EvidenceVersion, ContentSHA256: hashes.sha256, Facts: []filleradmission.Evidence{
			{ID: "media-usability", Claim: filleradmission.ClaimMediaUsability, Value: usability(measurement.Usable), Kind: filleradmission.KindDecoder, Source: "decoder:" + planned.CaseID, Location: measurement.Detail},
			{ID: "source-license", Claim: filleradmission.ClaimSourceLicense, Value: filleradmission.EligibilityEligible, Kind: filleradmission.KindSourcePolicy, Source: "rights:" + planned.CaseID},
		}, Signals: metadataSignals(item)}
		if measurement.Usable {
			caseDir := filepath.Join(stageRoot, fillercorpus.InventorySHA256([]byte(planned.CaseID)))
			frames, err := deriver.Frames(ctx, mediaPath, planned.SegmentStartMS, planned.SegmentStartMS+planned.SegmentDurationMS)
			if err != nil || len(frames) != 4 {
				return fillereval.Manifest{}, nil, fmt.Errorf("case %q requires exactly four bounded frames: %w", planned.CaseID, err)
			}
			for index, frame := range frames {
				cfg, _, err := image.DecodeConfig(bytes.NewReader(frame))
				if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 1920 {
					return fillereval.Manifest{}, nil, fmt.Errorf("case %q frame %d is invalid", planned.CaseID, index+1)
				}
				rel := filepath.ToSlash(filepath.Join(filepath.Base(caseDir), fmt.Sprintf("frame-%d.jpg", index+1)))
				if err := writeArtifact(filepath.Join(stageRoot, filepath.FromSlash(rel)), frame); err != nil {
					return fillereval.Manifest{}, nil, err
				}
				digest := fillercorpus.InventorySHA256(frame)
				outputBytes += int64(len(frame))
				packet.Signals = append(packet.Signals, fillerbakeoff.Signal{ID: fmt.Sprintf("frame-%d", index+1), Kind: string(filleradmission.KindFrame), Path: rel, SHA256: digest, Bytes: int64(len(frame)), Width: cfg.Width, Height: cfg.Height, AtMS: semanticAt(planned.SegmentStartMS, planned.SegmentDurationMS, index)})
			}
			video, err := deriver.Video(ctx, mediaPath, planned.VideoStartMS, planned.VideoStartMS+planned.VideoDurationMS)
			maxMeasuredVideoMS := min(mediatools.HostedVideoMaxDurationMS, planned.VideoDurationMS+1_000)
			if err != nil || len(video.Data) == 0 || video.DurationMS <= 0 || video.DurationMS > maxMeasuredVideoMS || video.Width <= 0 || video.Height <= 0 || video.Width > 1280 || video.Height > 720 || video.SHA256 != fillercorpus.InventorySHA256(video.Data) {
				return fillereval.Manifest{}, nil, fmt.Errorf("case %q direct-video derivative is invalid: %w", planned.CaseID, err)
			}
			videoRel := filepath.ToSlash(filepath.Join(filepath.Base(caseDir), "video.mp4"))
			if err := writeArtifact(filepath.Join(stageRoot, filepath.FromSlash(videoRel)), video.Data); err != nil {
				return fillereval.Manifest{}, nil, err
			}
			outputBytes += int64(len(video.Data))
			packet.Signals = append(packet.Signals, fillerbakeoff.Signal{ID: "video", Kind: string(filleradmission.KindVideo), Path: videoRel, SHA256: video.SHA256, Bytes: int64(len(video.Data)), DurationMS: video.DurationMS, Width: video.Width, Height: video.Height, AtMS: planned.VideoStartMS})
		}
		if outputBytes > opts.maxOutputBytes {
			return fillereval.Manifest{}, nil, fmt.Errorf("derivatives exceed aggregate byte ceiling")
		}
		if time.Since(started) > opts.maxWallTime {
			return fillereval.Manifest{}, nil, fmt.Errorf("wall-time ceiling exceeded")
		}
		evidenceDigest := fillerbakeoff.PacketSHA256(packet)
		itemRef := item.ItemURL
		if itemRef == "" {
			itemRef = "inventory:" + inventoryDigest + "#" + item.CaseID
		}
		license := item.LicenseURL
		if license == "" {
			license = strings.Join(item.RightsAssertions, "; ")
		}
		preparedCase := fillereval.Case{ID: item.CaseID, Split: planned.Split, Cluster: planned.Cluster, ContentSHA256: hashes.sha256, EvidenceSHA256: evidenceDigest, Source: item.Authority, License: license, Provenance: fillereval.MediaProvenance{
			Authority: item.Authority, Collection: strings.Join(item.Collection, ", "), ItemID: item.ItemID, ItemRef: itemRef, MetadataRetrievedAt: item.MetadataRetrievedAt, MetadataSHA256: item.MetadataSHA256,
			EvidenceRef: "inventory:" + inventoryDigest + "#" + item.CaseID, LicenseURL: item.LicenseURL, RightsStatement: strings.Join(item.RightsAssertions, "; ") + "; review basis: " + approval.Basis, RightsDecision: approval.Decision, RightsReviewerID: approval.ReviewerID, RightsReviewedAt: approval.ReviewedAt, Redistributable: approval.Redistributable,
			Creator: strings.Join(item.Creator, ", "), RequiredCredit: approval.RequiredCredit, Restrictions: slices.Clone(approval.Restrictions), SourceFilename: item.Representation.Name, SourceRef: sourceRef, SourceBytes: size, SegmentStartMS: planned.SegmentStartMS, SegmentDurationMS: planned.SegmentDurationMS,
		}}
		if err := fillerbakeoff.ValidatePacketAgainstCase(preparedCase, packet, plan.EvidenceVersion, stageRoot); err != nil {
			return fillereval.Manifest{}, nil, err
		}
		draft.Cases = append(draft.Cases, preparedCase)
		packets = append(packets, packet)
	}
	if len(seenCases) != len(byCase) || len(approved) != len(byCase) {
		return fillereval.Manifest{}, nil, fmt.Errorf("plan and approvals must cover every inventory case exactly once")
	}
	if splitCounts[fillereval.SplitDevelopment] == 0 || splitCounts[fillereval.SplitHoldout] == 0 {
		return fillereval.Manifest{}, nil, fmt.Errorf("preparation plan requires non-empty development and holdout splits")
	}
	sort.Slice(draft.Cases, func(i, j int) bool { return draft.Cases[i].ID < draft.Cases[j].ID })
	sort.Slice(packets, func(i, j int) bool { return packets[i].CaseID < packets[j].CaseID })
	if err := os.Rename(stageRoot, opts.derivativesRoot); err != nil {
		return fillereval.Manifest{}, nil, err
	}
	published = true
	return draft, packets, nil
}

func validateSliceGates(gates []fillereval.SliceGate, cases int) error {
	seen := map[string]struct{}{}
	for _, gate := range gates {
		if strings.TrimSpace(gate.Slice) == "" || gate.MinCases <= 0 || gate.MinCases > cases || gate.MinAccuracy <= 0 || gate.MinAccuracy > 1 || gate.MinAccuracyLower <= 0 || gate.MinAccuracyLower > 1 {
			return fmt.Errorf("slice gates require unique names, feasible case counts, and positive accuracy bounds at most one")
		}
		if _, duplicate := seen[gate.Slice]; duplicate {
			return fmt.Errorf("duplicate slice gate %q", gate.Slice)
		}
		seen[gate.Slice] = struct{}{}
	}
	return nil
}

func (d *realDeriver) Measure(ctx context.Context, path string, start, end int64) (mediaMeasurement, error) {
	p, err := filler.FFprobeNextTo(d.ffmpeg)(ctx, path)
	if err != nil {
		return mediaMeasurement{}, err
	}
	if end > p.DurationMs {
		return mediaMeasurement{}, fmt.Errorf("segment exceeds measured duration")
	}
	quality, err := mediatools.InspectQualityIn(ctx, d.ffmpeg, path, start, end, !p.Silent)
	if err != nil {
		return mediaMeasurement{}, err
	}
	black, silence := coverage(quality.Black, quality.DurationMs), coverage(quality.Silence, quality.DurationMs)
	usable := !p.NoVideo && !p.Silent && black < 90 && silence < 90
	return mediaMeasurement{DurationMS: p.DurationMs, Usable: usable, Detail: fmt.Sprintf("source_duration_ms=%d;segment_start_ms=%d;segment_duration_ms=%d;no_video=%t;no_audio=%t;black_percent=%d;silence_percent=%d", p.DurationMs, start, end-start, p.NoVideo, p.Silent, black, silence)}, nil
}

func (d *realDeriver) Frames(ctx context.Context, path string, start, end int64) ([][]byte, error) {
	return d.tools.KeyframesIn(ctx, path, start, end, 4)
}
func (d *realDeriver) Video(ctx context.Context, path string, start, end int64) (videoDerivative, error) {
	got, err := d.tools.HostedVideoIn(ctx, path, start, end)
	if err != nil {
		return videoDerivative{}, err
	}
	temp, err := os.CreateTemp("", "loomarr-corpus-video-*.mp4")
	if err != nil {
		return videoDerivative{}, err
	}
	name := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(name) }()
	if _, err = temp.Write(got.MP4); err != nil {
		return videoDerivative{}, err
	}
	if err = temp.Close(); err != nil {
		return videoDerivative{}, err
	}
	p, err := filler.FFprobeNextTo(d.ffmpeg)(ctx, name)
	if err != nil {
		return videoDerivative{}, err
	}
	return videoDerivative{Data: got.MP4, SHA256: got.SHA256, DurationMS: p.DurationMs, Width: p.Width, Height: p.Height}, nil
}

func coverage(spans []mediatools.Interval, duration int64) int64 {
	var total int64
	for _, s := range spans {
		total += s.EndMs - s.StartMs
	}
	if duration == 0 {
		return 0
	}
	return total * 100 / duration
}
func usability(ok bool) string {
	if ok {
		return filleradmission.UsabilityUsable
	}
	return filleradmission.UsabilityUnusable
}
func semanticAt(start, duration int64, index int) int64 {
	fractions := []float64{.05, 1.0 / 3, 2.0 / 3, .9}
	return start + int64(float64(duration)*fractions[index])
}

func metadataSignals(item fillercorpus.InventoryCase) []fillerbakeoff.Signal {
	values := []fillerbakeoff.Signal{{ID: "filename", Kind: string(filleradmission.KindFilename), Text: item.Representation.Name}}
	metadata, _ := json.Marshal(struct {
		Title   string   `json:"title"`
		Creator []string `json:"creator,omitempty"`
		Date    string   `json:"date,omitempty"`
	}{item.Title, item.Creator, item.Date})
	values = append(values, fillerbakeoff.Signal{ID: "source-metadata", Kind: string(filleradmission.KindUploaderMetadata), Text: string(metadata)})
	return values
}

func mediaPathFor(opts options, item fillercorpus.InventoryCase) (string, string, error) {
	if item.Representation.Transport == fillercorpus.TransportLocal {
		p, err := inside(opts.localRoot, item.Representation.Path)
		return p, "local:" + item.Representation.Path, err
	}
	name := fillercorpus.InventorySHA256([]byte(item.CaseID))[:16] + filepath.Ext(item.Representation.Name)
	p, err := inside(opts.remoteRoot, name)
	return p, item.Representation.URL, err
}
func inside(root, relative string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(rootReal, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootReal, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("media path escapes its declared root")
	}
	return candidate, nil
}

type mediaHashes struct{ sha256, sha1, md5 string }

func hashMedia(path string) (mediaHashes, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return mediaHashes{}, 0, err
	}
	defer func() { _ = f.Close() }()
	h256, h1, hm := sha256.New(), sha1.New(), md5.New()
	n, err := io.Copy(io.MultiWriter(h256, h1, hm), f)
	return mediaHashes{hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(h1.Sum(nil)), hex.EncodeToString(hm.Sum(nil))}, n, err
}
func matches(want, got string) bool { return want == "" || strings.EqualFold(want, got) }
func readStrictJSON(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 4<<20)
	for s.Scan() {
		if len(bytes.TrimSpace(s.Bytes())) == 0 {
			continue
		}
		var v T
		decoder := json.NewDecoder(bytes.NewReader(s.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&v); err != nil {
			return nil, err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return nil, fmt.Errorf("trailing JSON value in JSONL record")
		}
		out = append(out, v)
	}
	return out, s.Err()
}
func writeArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return atomicWrite(path, data)
}
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}
func writeJSONL[T any](path string, values []T) error {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	for _, v := range values {
		if err := e.Encode(v); err != nil {
			return err
		}
	}
	return atomicWrite(path, b.Bytes())
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-prepare-*")
	if err != nil {
		return err
	}
	name := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err = f.Chmod(0o600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
