package playoutcert

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

type SyntheticConfig struct {
	Channels []Channel
	FFmpeg   string
	Capacity int
	Grace    time.Duration
}

type SyntheticTarget struct {
	BaseURL     string
	AdminBearer string
	DeviceToken string

	server      *http.Server
	listener    net.Listener
	origin      *playout.Origin
	diagnostics *diagnostics.ProcessManager
	store       store.Store
	root        string
	closeOnce   sync.Once
}

func NewSyntheticTarget(ctx context.Context, config SyntheticConfig) (*SyntheticTarget, error) {
	if len(config.Channels) == 0 || len(config.Channels) > 1000 {
		return nil, errors.New("synthetic target requires 1..1000 Channels")
	}
	if config.Capacity <= 0 {
		config.Capacity = 4
	}
	if config.Grace <= 0 {
		config.Grace = 2 * time.Second
	}
	ffmpeg := strings.TrimSpace(config.FFmpeg)
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return nil, errors.New("synthetic target requires ffmpeg")
	}
	root, err := os.MkdirTemp("", "loomarr-playout-cert-")
	if err != nil {
		return nil, err
	}
	target := &SyntheticTarget{root: root}
	fail := func(err error) (*SyntheticTarget, error) { _ = target.Close(context.Background()); return nil, err }

	st, err := store.Open(ctx, "sqlite://"+filepath.Join(root, "loomarr.db"), true)
	if err != nil {
		return fail(err)
	}
	target.store = st
	for index, channel := range config.Channels {
		row := store.Channel{Channel: schedule.Channel{ID: channel.ID, Name: fmt.Sprintf("Synthetic %03d", index+1), Number: index + 1, Status: schedule.StatusLive}}
		row.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
		if _, err := st.SaveChannel(ctx, row); err != nil {
			return fail(err)
		}
	}

	source := filepath.Join(root, "source.mp4")
	if err := generateSyntheticSource(ctx, ffmpeg, source); err != nil {
		return fail(err)
	}
	library, err := prepared.NewLibrary(filepath.Join(root, "prepared"))
	if err != nil {
		return fail(err)
	}
	rendition := prepared.RenditionContract{
		VideoCodec: "h264", VideoProfile: "high", VideoLevel: "4.1", PixelFormat: "yuv420p", HDR: "sdr",
		AudioCodec: "aac", AudioLayout: "stereo", Width: 320, Height: 180, FrameRate: 25,
		VideoBitrateKbps: 600, AudioBitrateKbps: 96, SegmentDurationMS: 1000, PackagingVersion: prepared.CurrentPackagingVersion,
	}
	preparedSpecs := make(map[string]prepared.Specification)
	preparedIndexes := preparedChannelIndexes(config.Channels)
	if len(preparedIndexes) > 0 {
		first := config.Channels[preparedIndexes[0]]
		firstSpec := prepared.Specification{SourceFingerprint: "synthetic-" + first.ID, Rendition: rendition}
		packager := prepared.NewFFmpegPackager(ffmpeg)
		template, publishErr := library.Publish(ctx, firstSpec, func(buildCtx context.Context, workspace string) (prepared.Output, error) {
			return packager.Package(buildCtx, workspace, prepared.LocalInput(source), 0, rendition)
		})
		if publishErr != nil {
			return fail(publishErr)
		}
		preparedSpecs[first.ID] = firstSpec
		for _, index := range preparedIndexes[1:] {
			channel := config.Channels[index]
			spec := prepared.Specification{SourceFingerprint: "synthetic-" + channel.ID, Rendition: rendition}
			_, publishErr = library.Publish(ctx, spec, func(_ context.Context, workspace string) (prepared.Output, error) {
				for _, name := range template.Files {
					from, to := filepath.Join(template.Directory, name), filepath.Join(workspace, name)
					if linkErr := os.Link(from, to); linkErr != nil {
						if copyErr := copyRegularFile(from, to); copyErr != nil {
							return prepared.Output{}, copyErr
						}
					}
				}
				return prepared.Output{Files: append([]string(nil), template.Files...)}, nil
			})
			if publishErr != nil {
				return fail(publishErr)
			}
			preparedSpecs[channel.ID] = spec
		}
	}

	admin, err := randomCredential()
	if err != nil {
		return fail(err)
	}
	device, err := randomCredential()
	if err != nil {
		return fail(err)
	}
	target.AdminBearer, target.DeviceToken = admin, device
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	target.listener = listener
	target.BaseURL = "http://" + listener.Addr().String()

	logger := slog.New(slog.DiscardHandler)
	processManager := diagnostics.NewProcessManager(st, nil, diagnostics.ProcessOptions{
		OutputDir: filepath.Join(root, "diagnostics"), InstanceID: "synthetic-cert",
	})
	target.diagnostics = processManager
	processLog := diagnostics.NewProcessLog(st, diagnostics.ProcessReadOptions{OutputDir: filepath.Join(root, "diagnostics")})
	preparedResolver := syntheticPreparedResolver{specs: preparedSpecs, started: time.Now().Add(-2 * time.Second)}
	preparedCount := len(preparedIndexes)
	preparedStatus := syntheticPreparedStatus{count: preparedCount}
	preparedOrigin := playout.NewPreparedOrigin(library, preparedResolver)
	preparedBlock := preparedOrigin.MPEGTSBlockSource(ffmpeg, logger, processManager)
	liveResolver := syntheticLiveResolver{source: source}

	var manager *playout.Manager
	spawner := func(spawnCtx context.Context, channelID string, plan playout.EncodePlan) (*playout.Process, error) {
		sourceForParent := syntheticBlockSource(target.BaseURL, device, preparedBlock, manager)
		return playout.BlockSpawner(ffmpeg, sourceForParent, logger, processManager)(spawnCtx, channelID, plan)
	}
	manager = playout.NewManager(spawner, func() int { return config.Capacity }, config.Grace, logger).
		WithCostEstimator(func(estimateCtx context.Context, channelID string, plan playout.EncodePlan) int {
			ready, readyErr := preparedOrigin.MPEGTSReady(estimateCtx, channelID, plan)
			if readyErr == nil && ready {
				return 0
			}
			return plan.EstimatedCost()
		})
	recorder := metrics.New(metrics.Options{Version: "synthetic", Revision: "synthetic", Database: "sqlite"})
	manager.WithObserver(recorder)
	origin := playout.NewOrigin(playout.OriginDependencies{Prepared: preparedOrigin, LiveSessions: manager, Observer: recorder})
	target.origin = origin
	handler := api.Router(logger, api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(admin), Log: logger, Metrics: recorder,
		PlayoutSecret: func() string { return device }, Playout: origin, PlayoutObserver: manager,
		PreparedObserver: preparedStatus,
		PlayoutResolver:  liveResolver,
		PlayoutEncoder: func(encodeCtx context.Context, args []string, progress func(playout.Progress)) (*playout.Process, error) {
			spec, _ := diagnostics.ProcessSpecFromContext(encodeCtx)
			return playout.StartObserved(encodeCtx, ffmpeg, args, logger, progress, processManager, spec)
		},
		DiagnosticProcesses: processLog,
		LiveConfig: func(key string) string {
			switch key {
			case "server.public_url":
				return target.BaseURL
			case "playout.backend":
				return schedule.PlayoutBackendInternal
			default:
				return ""
			}
		},
	})
	syntheticHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go test binaries do not carry the repository VCS stamp. The isolated
		// target therefore owns an explicit synthetic identity while every real
		// target still has to return its linker-stamped Loomarr revision.
		if r.URL.Path == "/v1/system/version" && r.Method == http.MethodGet && r.Header.Get("Authorization") == "Bearer "+admin {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "synthetic", "commit": strings.Repeat("0", 40), "ready": true,
			})
			return
		}
		handler.ServeHTTP(w, r)
	})
	target.server = &http.Server{Handler: syntheticHandler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = target.server.Serve(listener) }()
	return target, nil
}

func (t *SyntheticTarget) Close(ctx context.Context) error {
	var result error
	t.closeOnce.Do(func() {
		if t.origin != nil {
			t.origin.Quiesce()
		}
		if t.server != nil {
			result = t.server.Shutdown(ctx)
		} else if t.listener != nil {
			result = t.listener.Close()
		}
		if t.diagnostics != nil {
			if err := t.diagnostics.Close(ctx); result == nil {
				result = err
			}
		}
		if t.store != nil {
			if err := t.store.Close(); result == nil {
				result = err
			}
		}
		if t.root != "" {
			if err := os.RemoveAll(t.root); result == nil {
				result = err
			}
		}
	})
	return result
}

type syntheticPreparedResolver struct {
	specs   map[string]prepared.Specification
	started time.Time
}

type syntheticPreparedStatus struct{ count int }

func (s syntheticPreparedStatus) Status() prepared.PlannerStatus {
	return prepared.PlannerStatus{
		Available: true, LastRunAt: time.Now(),
		Readiness: prepared.ReadinessSummary{
			Channels: s.count, ReadyChannels: s.count,
			ScheduledBindings: s.count, ReadyBindings: s.count,
		},
	}
}

func (r syntheticPreparedResolver) ResolvePrepared(_ context.Context, request playout.TuneRequest) (playout.PreparedWindow, bool, error) {
	spec, ok := r.specs[request.ChannelID]
	if !ok {
		return playout.PreparedWindow{}, false, nil
	}
	now := time.Now()
	offset := now.Sub(r.started)
	ends := r.started.Add(60 * time.Second)
	if offset < 0 || !now.Before(ends) {
		return playout.PreparedWindow{}, false, nil
	}
	return playout.PreparedWindow{Current: playout.PreparedAiring{
		Specification: spec, StartedAt: r.started, Offset: offset,
		Identity: playout.AiringIdentity{StartedAt: r.started, EndsAt: ends, Kind: schedule.SlotProgram, ContentID: request.ChannelID, ScheduleBlockID: "synthetic-block"},
	}}, true, nil
}

type syntheticLiveResolver struct{ source string }

func (r syntheticLiveResolver) AiringNow(_ context.Context, channelID string) (playout.Airing, string, error) {
	started := time.Now().Add(-time.Second)
	return playout.Airing{StartedAt: started, Identity: "live-" + channelID, Kind: schedule.SlotProgram, LibraryItemID: channelID, Title: "Synthetic", Remaining: 50 * time.Second}, r.source, nil
}
func (syntheticLiveResolver) Profile(context.Context) playout.Profile {
	p := playout.DefaultProfile()
	p.Width = 320
	p.Height = 180
	p.VideoBitrate = 600
	p.AudioBitrate = 96
	p.Encoder = playout.EncoderSoftware
	return p
}
func (syntheticLiveResolver) AudioTrackFor(context.Context, string, string, string) int { return 0 }
func (syntheticLiveResolver) Tracks(context.Context, string) (playout.MediaTracks, error) {
	return playout.MediaTracks{}, nil
}
func (syntheticLiveResolver) PlanFor(context.Context, string, playout.EncodePlan) (playout.CopyPlan, playout.MediaFormat) {
	return playout.CopyPlan{}, playout.MediaFormat{}
}
func (syntheticLiveResolver) ChannelCodec(context.Context, string) string { return "h264" }

func syntheticBlockSource(base, device string, preparedSource playout.BlockSource, manager *playout.Manager) playout.BlockSource {
	var broadcast string
	return func(ctx context.Context, channelID string, plan playout.EncodePlan) (playout.Block, error) {
		if preparedSource != nil {
			block, err := preparedSource(ctx, channelID, plan)
			if err == nil && block.Content != nil {
				if manager != nil && !manager.AdmitProgram(channelID, plan, false) {
					_ = block.Content.Close()
					return playout.Block{}, playout.ErrAtCapacity
				}
				format, valid := playout.ParseBroadcastFormat(block.Format.String())
				if valid && (broadcast == "" || broadcast == format.String()) {
					broadcast = format.String()
					block.Format = format
					return block, nil
				}
				_ = block.Content.Close()
			}
			if ctx.Err() != nil {
				return playout.Block{}, ctx.Err()
			}
		}
		query := url.Values{"token": {device}, "plan": {plan.String()}}
		if broadcast != "" {
			query.Set(api.PlayoutBroadcastFormatQuery, broadcast)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/playout/program/"+url.PathEscape(channelID)+"?"+query.Encode(), nil)
		if err != nil {
			return playout.Block{}, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return playout.Block{}, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return playout.Block{}, fmt.Errorf("program endpoint status %d", resp.StatusCode)
		}
		format, ok := playout.ParseBroadcastFormat(resp.Header.Get(api.PlayoutBroadcastFormatHeader))
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint format missing")
		}
		if broadcast == "" {
			broadcast = format.String()
		} else if broadcast != format.String() {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint format changed")
		}
		identity, ok := api.ParsePlayoutAiringIdentity(resp.Header)
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint identity missing")
		}
		return playout.Block{Content: resp.Body, Identity: identity, Format: format}, nil
	}
}

func generateSyntheticSource(ctx context.Context, ffmpeg, output string) error {
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=60", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=60", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25", "-c:a", "aac", "-b:a", "96k", output)
	if err := cmd.Run(); err != nil {
		return errors.New("generate deterministic synthetic media")
	}
	return nil
}

func randomCredential() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func copyRegularFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

var _ api.PlayoutResolver = syntheticLiveResolver{}
var _ playout.PreparedResolver = syntheticPreparedResolver{}
