package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SupportBundleFormat       = "loomarr.support-bundle.v1"
	maxBundleEvents           = 2000
	maxBundleProcesses        = 50
	maxBundleProcessOutputs   = 8
	maxBundleUncompressed     = 16 << 20
	maxBundleAssemblyDuration = 30 * time.Second
)

var (
	ErrInvalidBundleSelection = errors.New("invalid support bundle selection")
	ErrBundleTooLarge         = errors.New("support bundle exceeds safety bound")
)

type BundleSelection struct {
	From              int64  `json:"from" minimum:"0"`
	To                int64  `json:"to" minimum:"0"`
	Events            bool   `json:"events"`
	Processes         bool   `json:"processes"`
	ProcessOutput     bool   `json:"processOutput"`
	RequestID         string `json:"requestId,omitempty" maxLength:"128"`
	PlaybackSessionID string `json:"playbackSessionId,omitempty" maxLength:"128"`
	ChannelID         string `json:"channelId,omitempty" maxLength:"128"`
	ScheduleBlockID   string `json:"scheduleBlockId,omitempty" maxLength:"128"`
	JobID             string `json:"jobId,omitempty" maxLength:"128"`
	ProcessRunID      string `json:"processRunId,omitempty" maxLength:"128"`
}

type BundleEntry struct {
	Name              string `json:"name"`
	UncompressedBytes int64  `json:"uncompressedBytes"`
}

// BundleBuild is the diagnostics-owned build projection. The composition root maps the binary's
// build information into it so this widely imported module remains a dependency-free foundation.
type BundleBuild struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	BuiltAt string `json:"builtAt,omitempty"`
	Dirty   bool   `json:"dirty,omitempty"`
}

type BundleCounts struct {
	Events                  int    `json:"events"`
	EventsOmittedAtLeast    int    `json:"eventsOmittedAtLeast"`
	Processes               int    `json:"processes"`
	ProcessesOmittedAtLeast int    `json:"processesOmittedAtLeast"`
	ProcessOutputs          int    `json:"processOutputs"`
	ProcessOutputsOmitted   int    `json:"processOutputsOmitted"`
	EventRecorderDrops      uint64 `json:"eventRecorderDrops"`
	DiscardedLines          int64  `json:"discardedProcessLines"`
	Redactions              int    `json:"redactions"`
}

type BundleManifest struct {
	FormatVersion     string          `json:"formatVersion"`
	GeneratedAt       int64           `json:"generatedAt"`
	Selection         BundleSelection `json:"selection"`
	EffectiveFrom     int64           `json:"effectiveFrom"`
	EffectiveTo       int64           `json:"effectiveTo"`
	Loomarr           BundleBuild     `json:"loomarr"`
	ClientVersions    []string        `json:"clientVersions"`
	Entries           []BundleEntry   `json:"entries"`
	Counts            BundleCounts    `json:"counts"`
	TruncationReasons []string        `json:"truncationReasons"`
	UncompressedBytes int64           `json:"uncompressedBytes"`
	FinalArchiveBytes int64           `json:"finalArchiveBytes"`
}

type BundlePreview struct {
	Manifest       BundleManifest `json:"manifest"`
	EstimatedBytes int64          `json:"estimatedBytes"`
}

type BundleResult struct {
	Manifest BundleManifest
	Content  []byte
}

type BundleOptions struct {
	Events    *EventLog
	Processes *ProcessLog
	Health    func() HealthReport
	Build     func() BundleBuild
	Now       func() time.Time
}

type BundleService struct {
	events    *EventLog
	processes *ProcessLog
	health    func() HealthReport
	build     func() BundleBuild
	now       func() time.Time
}

func NewBundleService(opts BundleOptions) *BundleService {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Build == nil {
		opts.Build = func() BundleBuild { return BundleBuild{Version: "dev"} }
	}
	return &BundleService{events: opts.Events, processes: opts.Processes, health: opts.Health, build: opts.Build, now: opts.Now}
}

func (s *BundleService) Preview(ctx context.Context, selection BundleSelection) (BundlePreview, error) {
	selected, err := s.selectEvidence(ctx, selection)
	if err != nil {
		return BundlePreview{}, err
	}
	return BundlePreview{Manifest: selected.manifest, EstimatedBytes: selected.manifest.UncompressedBytes}, nil
}

func (s *BundleService) Build(ctx context.Context, selection BundleSelection) (BundleResult, error) {
	selected, err := s.selectEvidence(ctx, selection)
	if err != nil {
		return BundleResult{}, err
	}
	manifest := selected.manifest
	for range 10 {
		content, writeErr := writeBundle(manifest, selected.entries)
		if writeErr != nil {
			return BundleResult{}, writeErr
		}
		if len(content) > maxBundleUncompressed {
			return BundleResult{}, ErrBundleTooLarge
		}
		finalBytes := int64(len(content))
		if finalBytes == manifest.FinalArchiveBytes {
			return BundleResult{Manifest: manifest, Content: content}, nil
		}
		manifest.FinalArchiveBytes = finalBytes
	}
	return BundleResult{}, errors.New("support bundle manifest size did not stabilize")
}

type bundleEvidence struct {
	manifest BundleManifest
	entries  []bundleFile
}

type bundleFile struct {
	name    string
	content []byte
}

func (s *BundleService) selectEvidence(parent context.Context, selection BundleSelection) (bundleEvidence, error) {
	if s == nil || s.events == nil || s.processes == nil {
		return bundleEvidence{}, errors.New("support bundle service unavailable")
	}
	selection, err := validateBundleSelection(selection)
	if err != nil {
		return bundleEvidence{}, err
	}
	ctx, cancel := context.WithTimeout(parent, maxBundleAssemblyDuration)
	defer cancel()
	manifestSelection, redactions := defensivelyRedactSelection(selection)
	manifest := BundleManifest{FormatVersion: SupportBundleFormat, GeneratedAt: s.now().UnixMilli(), Selection: manifestSelection,
		EffectiveFrom: selection.From, EffectiveTo: selection.To, Loomarr: s.build(), Entries: []BundleEntry{}, ClientVersions: []string{}, TruncationReasons: []string{}}
	entries := []bundleFile{}
	if selection.Events {
		items, dropped, truncated, err := s.selectEvents(ctx, selection)
		if err != nil {
			return bundleEvidence{}, err
		}
		manifest.Counts.Events, manifest.Counts.EventRecorderDrops = len(items), dropped
		if truncated {
			manifest.Counts.EventsOmittedAtLeast = 1
			manifest.TruncationReasons = append(manifest.TruncationReasons, "events_limit")
		}
		versions := map[string]struct{}{}
		var data bytes.Buffer
		enc := json.NewEncoder(&data)
		for _, item := range items {
			clean, count := defensivelyRedactEvent(item)
			redactions += count
			if v, ok := clean.Attributes["client_version"].(string); ok && v != "" {
				versions[string(clean.Source)+":"+v] = struct{}{}
			}
			if err := enc.Encode(clean); err != nil {
				return bundleEvidence{}, fmt.Errorf("encode bundle event: %w", err)
			}
		}
		for version := range versions {
			manifest.ClientVersions = append(manifest.ClientVersions, version)
		}
		sort.Strings(manifest.ClientVersions)
		entries = append(entries, bundleFile{name: "events.ndjson", content: data.Bytes()})
	}
	if selection.Processes || selection.ProcessOutput {
		page, err := s.processes.Query(ctx, ProcessQuery{From: selection.From, To: selection.To, Limit: maxBundleProcesses, ChannelID: selection.ChannelID, JobID: selection.JobID})
		if err != nil {
			return bundleEvidence{}, err
		}
		runs := page.Items
		if selection.ProcessRunID != "" {
			runs = runs[:0]
			detail, getErr := s.processes.Get(ctx, selection.ProcessRunID)
			if getErr == nil && detail.Run.StartedAt >= selection.From && detail.Run.StartedAt <= selection.To {
				runs = append(runs, detail.Run)
			}
		}
		manifest.Counts.Processes = len(runs)
		if page.NextCursor != "" {
			manifest.Counts.ProcessesOmittedAtLeast = 1
			manifest.TruncationReasons = appendUnique(manifest.TruncationReasons, "processes_limit")
		}
		cleanRuns := make([]ProcessRunView, 0, len(runs))
		for _, run := range runs {
			clean, count := defensivelyRedactProcess(run)
			redactions += count
			cleanRuns = append(cleanRuns, clean)
		}
		index, _ := json.MarshalIndent(cleanRuns, "", "  ")
		entries = append(entries, bundleFile{name: "processes/index.json", content: append(index, '\n')})
		for i, run := range runs {
			name := fmt.Sprintf("processes/%04d.json", i+1)
			meta, _ := json.MarshalIndent(cleanRuns[i], "", "  ")
			entries = append(entries, bundleFile{name: name, content: append(meta, '\n')})
			if selection.ProcessOutput && i < maxBundleProcessOutputs {
				output, outputErr := s.processes.Output(ctx, run.ID)
				if outputErr == nil {
					cleanOutput, count := defensiveText(string(output.Content))
					redactions += count
					manifest.Counts.ProcessOutputs++
					manifest.Counts.DiscardedLines += output.DiscardedLines
					if output.Truncated {
						manifest.TruncationReasons = appendUnique(manifest.TruncationReasons, "process_output_retention")
					}
					entries = append(entries, bundleFile{name: fmt.Sprintf("processes/%04d.log", i+1), content: []byte(cleanOutput)})
				} else {
					manifest.Counts.ProcessOutputsOmitted++
				}
			}
		}
		if selection.ProcessOutput && len(runs) > maxBundleProcessOutputs {
			manifest.Counts.ProcessOutputsOmitted += len(runs) - maxBundleProcessOutputs
			manifest.TruncationReasons = appendUnique(manifest.TruncationReasons, "process_output_limit")
		}
	}
	health := HealthReport{State: HealthUnhealthy, Checks: []HealthCheck{}}
	if s.health != nil {
		health = s.health()
	}
	for index := range health.Checks {
		clean, count := defensiveText(health.Checks[index].Detail)
		health.Checks[index].Detail = clean
		redactions += count
	}
	system, _ := json.MarshalIndent(struct {
		Build  BundleBuild  `json:"build"`
		Health HealthReport `json:"currentHealth"`
	}{manifest.Loomarr, health}, "", "  ")
	entries = append([]bundleFile{{name: "system.json", content: append(system, '\n')}}, entries...)
	manifest.Counts.Redactions = redactions
	for _, entry := range entries {
		manifest.Entries = append(manifest.Entries, BundleEntry{Name: entry.name, UncompressedBytes: int64(len(entry.content))})
		manifest.UncompressedBytes += int64(len(entry.content))
	}
	if manifest.UncompressedBytes > maxBundleUncompressed {
		return bundleEvidence{}, ErrBundleTooLarge
	}
	return bundleEvidence{manifest: manifest, entries: entries}, nil
}

func validateBundleSelection(s BundleSelection) (BundleSelection, error) {
	if s.From < 0 || s.To <= s.From || s.To-s.From > maxSelectionWindow.Milliseconds() {
		return s, fmt.Errorf("%w: from and to must define a window of at most 24 hours", ErrInvalidBundleSelection)
	}
	if !s.Events && !s.Processes && !s.ProcessOutput {
		return s, fmt.Errorf("%w: select at least one category", ErrInvalidBundleSelection)
	}
	if s.ProcessOutput && !s.Processes {
		return s, fmt.Errorf("%w: Process output requires Process metadata", ErrInvalidBundleSelection)
	}
	for name, value := range map[string]string{"requestId": s.RequestID, "playbackSessionId": s.PlaybackSessionID, "channelId": s.ChannelID, "scheduleBlockId": s.ScheduleBlockID, "jobId": s.JobID, "processRunId": s.ProcessRunID} {
		if len(value) > maxFilterBytes {
			return s, fmt.Errorf("%w: %s cannot exceed 128 bytes", ErrInvalidBundleSelection, name)
		}
	}
	return s, nil
}

func (s *BundleService) selectEvents(ctx context.Context, selection BundleSelection) ([]EventView, uint64, bool, error) {
	items := make([]EventView, 0, maxBundleEvents)
	cursor := ""
	var dropped uint64
	for len(items) < maxBundleEvents {
		page, err := s.events.Query(ctx, EventQuery{From: selection.From, To: selection.To, Limit: maxEventLimit, Cursor: cursor,
			RequestID: selection.RequestID, PlaybackSessionID: selection.PlaybackSessionID, ChannelID: selection.ChannelID,
			ScheduleBlockID: selection.ScheduleBlockID, JobID: selection.JobID, ProcessRunID: selection.ProcessRunID})
		if err != nil {
			return nil, 0, false, err
		}
		dropped = page.Dropped
		items = append(items, page.Items...)
		if page.NextCursor == "" {
			return items, dropped, false, nil
		}
		cursor = page.NextCursor
	}
	return items, dropped, cursor != "", nil
}

func writeBundle(manifest BundleManifest, entries []bundleFile) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	all := append([]bundleFile{{name: "manifest.json", content: append(manifestJSON, '\n')}}, entries...)
	for _, entry := range all {
		header := &zip.FileHeader{
			Name: entry.name, Method: zip.Deflate,
			Modified: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(entry.content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func defensivelyRedactEvent(event EventView) (EventView, int) {
	count := 0
	for _, target := range []*string{&event.Message, &event.RequestID, &event.PlaybackSessionID, &event.ChannelID, &event.ScheduleBlockID, &event.JobID, &event.ProcessRunID, &event.ActorID, &event.InstanceID} {
		clean, n := defensiveText(*target)
		*target = clean
		count += n
	}
	event.Attributes, count = defensiveMap(event.Attributes, count)
	return event, count
}

func defensivelyRedactProcess(run ProcessRunView) (ProcessRunView, int) {
	count := 0
	for _, target := range []*string{&run.ParentRunID, &run.InstanceID, &run.ChannelID, &run.Target, &run.ScheduleBlockID, &run.JobID, &run.Executable, &run.ExecutableVersion, &run.TerminationReason, &run.FirstError, &run.LastError} {
		clean, n := defensiveText(*target)
		*target = clean
		count += n
	}
	return run, count
}

func defensivelyRedactSelection(selection BundleSelection) (BundleSelection, int) {
	count := 0
	for _, target := range []*string{&selection.RequestID, &selection.PlaybackSessionID, &selection.ChannelID, &selection.ScheduleBlockID, &selection.JobID, &selection.ProcessRunID} {
		clean, replacements := defensiveText(*target)
		*target = clean
		count += replacements
	}
	return selection, count
}

func defensiveMap(values map[string]any, count int) (map[string]any, int) {
	out := make(map[string]any, len(values))
	for key, value := range values {
		if sensitiveKey(key) {
			out[key] = redacted
			if value != redacted {
				count++
			}
			continue
		}
		switch typed := value.(type) {
		case string:
			out[key], count = defensiveTextWithCount(typed, count)
		case map[string]any:
			out[key], count = defensiveMap(typed, count)
		case []any:
			items := make([]any, len(typed))
			for i, item := range typed {
				if text, ok := item.(string); ok {
					items[i], count = defensiveTextWithCount(text, count)
				} else {
					items[i] = item
				}
			}
			out[key] = items
		default:
			out[key] = value
		}
	}
	return out, count
}

func defensiveTextWithCount(value string, count int) (string, int) {
	clean, n := defensiveText(value)
	return clean, count + n
}
func defensiveText(value string) (string, int) {
	clean := sanitizeProcessOutput(value)
	if clean != strings.TrimSpace(value) {
		return clean, 1
	}
	return clean, 0
}
func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
