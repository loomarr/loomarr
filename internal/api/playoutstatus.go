package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/playout"
)

// Playout status (§9.1 V47) — the one endpoint the dashboard's Playout panel reads: the live
// channels on air plus the shared-GPU picture, which together answer "what is playing, and why
// would it be slow or black?".
//
// Several distinct failures all present to a viewer as a black frame: a codec the target can't
// decode, a hardware encode starved of VRAM by the resident LLM (§8.2), a transcode running below
// realtime, a channel with no session at all. Telling them apart means correlating the live encoders
// (speed is the tell — a sustained sub-1.0× is a stall), the GPU + its VRAM, and the resident LLM
// sharing that VRAM. This composes exactly those into one read-only picture. It changes nothing —
// safe to poll. scripts/playout-diag.sh is the shell-level twin for a support session.

// PlayoutHealth is a channel's verdict — the traffic light the dashboard renders.
type PlayoutHealth string

const (
	// HealthOK: the encoder is comfortably keeping up (speed well above realtime, or a copy).
	HealthOK PlayoutHealth = "ok"
	// HealthDegraded: near realtime — playing now, but with little margin; a spike will stall it.
	HealthDegraded PlayoutHealth = "degraded"
	// HealthStalled: below realtime — the encoder is losing to wall-clock and the viewer buffers.
	HealthStalled PlayoutHealth = "stalled"
)

// stallSpeed / degradedSpeed are the realtime multiples that divide the verdicts. A copy reports a
// meaningless speed (it does no encoding), so a copy is always OK regardless of the number.
//
// 1.0× is exactly realtime — at or below it the encoder produces less than one second of video per
// wall-clock second and the buffer drains. 1.3× leaves ~30% headroom for the inevitable slow moment
// (a complex scene, a filesystem hiccup); below that a channel is one bad beat from stalling.
const (
	stallSpeed    = 1.0
	degradedSpeed = 1.3
)

// ChannelHealth is one (channel, target)'s health row.
//
// It carries everything the merged dashboard Playout panel renders per channel, so the panel reads
// ONE endpoint: identity (number/name for display, id for detail), the live throughput (viewers,
// speed, buffer), the resolved encoder, the cold-start latency, and the health verdict. Number/Name
// are resolved in the handler from the store — the playout layer that produces the underlying
// SessionStat stays channel-agnostic (id-only) by design.
type ChannelHealth struct {
	ChannelID string `json:"channelId"`
	// Number/Name are the human identity — "#3 · 1980s Action Heroes" — shown instead of the raw id
	// (which the UI keeps behind a detail affordance). 0/"" when the channel is not found in the store
	// (a just-deleted channel still finishing its stream), so the row degrades to the id alone.
	Number int    `json:"number" doc:"Channel number for display (0 = unknown)"`
	Name   string `json:"name" doc:"Channel name for display (empty = unknown)"`
	Target string `json:"target" doc:"Codec audience: browser|mediaserver (§9.1 V47)"`
	// Viewers/BufferedMS are the live throughput, folded in from the session so the merged panel needs
	// only this endpoint (previously split across GET /v1/playout/sessions).
	Viewers    int           `json:"viewers" doc:"How many clients are attached"`
	BufferedMS int64         `json:"bufferedMs" doc:"Encoded lead over realtime in ms; negative = falling behind"`
	Encoder    string        `json:"encoder" doc:"Resolved encoder, or 'copy' when direct-playing"`
	Hardware   bool          `json:"hardware" doc:"Whether the encoder is hardware-accelerated"`
	Mode       string        `json:"mode" doc:"direct-play (copy) | transcode"`
	Speed      float64       `json:"speed" doc:"Realtime multiple; <1.0 means the channel will stutter"`
	Health     PlayoutHealth `json:"health" doc:"ok | degraded | stalled"`
	Reason     string        `json:"reason" doc:"One line an operator can act on"`
	// ColdStartMS is how long this channel took to first frame — the black-screen window a viewer
	// waited through (§9.1 V47). The one number that tracks the black-screen symptom; 0 = not measured.
	ColdStartMS int64 `json:"coldStartMs" doc:"Time to first frame in ms — the black-screen window; 0 if unmeasured"`
}

// PlayoutGPU is the shared-GPU header — the contention picture the encoder rows alone can't show.
type PlayoutGPU struct {
	Name       string  `json:"name,omitempty"`
	VRAMGiB    float64 `json:"vramGiB,omitempty" doc:"Total detected GPU VRAM (0 = unknown/none)"`
	LLMModel   string  `json:"llmModel,omitempty" doc:"Resident LLM sharing this GPU, if any (§8.2)"`
	LLMVRAMGiB float64 `json:"llmVramGiB,omitempty" doc:"VRAM the resident LLM holds — unavailable to encoders"`
	Contended  bool    `json:"contended" doc:"A model is resident AND channels are transcoding on hardware"`
}

// PlayoutStatus is the whole health picture.
type PlayoutStatus struct {
	Running  bool            `json:"running" doc:"Internal playout is wired (false on a Tunarr-only install)"`
	GPU      PlayoutGPU      `json:"gpu"`
	Channels []ChannelHealth `json:"channels"`
}

type playoutStatusOutput struct {
	Body PlayoutStatus
}

// registerPlayoutStatus mounts GET /v1/playout/status (§9.1 V47).
func (s *Server) registerPlayoutStatus(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-playout-status", Method: http.MethodGet, Path: "/v1/playout/status",
		Summary: "Playout status — why a channel is (or isn't) playing (admin)",
		Description: "A read-only health projection of live internal playout: per (channel, target) " +
			"encoder + realtime-speed verdict (ok|degraded|stalled), plus the GPU/VRAM and resident-LLM " +
			"contention picture that a black-frame diagnosis needs. Admin only. Changes nothing.",
		Tags: []string{"dashboard"},
	}, RoleAdmin), s.getPlayoutStatus)
}

func (s *Server) getPlayoutStatus(ctx context.Context, _ *struct{}) (*playoutStatusOutput, error) {
	return &playoutStatusOutput{Body: s.playoutStatus(ctx, time.Now())}, nil
}

// playoutStatus builds the health picture from live state. Best-effort on the GPU/LLM header — a
// probe that fails degrades to an empty header, never a failed request, because the channel health
// (the load-bearing part) does not depend on it.
func (s *Server) playoutStatus(ctx context.Context, now time.Time) PlayoutStatus {
	if s.playoutSessions == nil {
		return PlayoutStatus{Channels: []ChannelHealth{}} // Tunarr-only: not our job
	}

	// Resolve channel id → human identity (number + name) once, from the store — the playout layer
	// only knows ids (agnostic by design), so the display name is joined here. Best-effort: a store
	// error leaves rows with number 0 / name "", and the UI falls back to the id.
	names := s.channelIdentities(ctx)

	channels := make([]ChannelHealth, 0)
	hwTranscoding := false
	for _, st := range s.playoutSessions.Stats(now) {
		h := channelHealthFrom(st)
		if id, ok := names[h.ChannelID]; ok {
			h.Number, h.Name = id.number, id.name
		}
		if h.Mode == "transcode" && st.Hardware {
			hwTranscoding = true
		}
		channels = append(channels, h)
	}

	gpu := s.statusGPU(ctx)
	// Contention is the specific bad combination: a model is holding VRAM AND channels are
	// transcoding on the GPU. That is the state that starved the encoders and blacked channels
	// (§8.2) — worth flagging distinctly from "a model happens to be loaded" (harmless when idle).
	gpu.Contended = gpu.LLMVRAMGiB > 0 && hwTranscoding

	return PlayoutStatus{Running: true, GPU: gpu, Channels: channels}
}

// channelIdentity is a channel's human-facing labels for a playout row.
type channelIdentity struct {
	number int
	name   string
}

// channelIdentities maps channel id → number + name from the store, so a playout row can show
// "#3 · 1980s Action Heroes" instead of the raw id (which the UI keeps behind a detail affordance).
// Best-effort: a nil store or a read error returns an empty map, and rows fall back to the id.
func (s *Server) channelIdentities(ctx context.Context) map[string]channelIdentity {
	out := map[string]channelIdentity{}
	if s.store == nil {
		return out
	}
	chans, err := s.store.ListChannels(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Debug("playout status: channel identity lookup failed", "err", err)
		}
		return out
	}
	for _, c := range chans {
		out[c.ID] = channelIdentity{number: c.Number, name: c.Name}
	}
	return out
}

// channelHealthFrom turns one raw SessionStat into a health row with a verdict + reason.
func channelHealthFrom(st playout.SessionStat) ChannelHealth {
	copying := st.Encoder == "copy" || st.Encoder == ""
	mode := "transcode"
	if copying {
		mode = "direct-play"
	}

	health, reason := verdict(copying, st.Speed, st.Hardware)
	return ChannelHealth{
		ChannelID: st.ChannelID, Target: st.Target,
		Viewers: st.Viewers, BufferedMS: st.BufferedMS,
		Encoder: st.Encoder, Hardware: st.Hardware,
		Mode: mode, Speed: st.Speed, Health: health, Reason: reason,
		ColdStartMS: st.ColdStartMS,
	}
}

// verdict maps (copy?, speed, hardware?) → a health level and an actionable reason.
//
// A COPY is always OK: it does no encoding, so its reported speed is meaningless (it measures
// remux throughput, which is not the constraint). Only a transcode's speed is a real signal.
func verdict(copying bool, speed float64, hardware bool) (PlayoutHealth, string) {
	if copying {
		return HealthOK, "Direct play — no transcode, no GPU."
	}
	switch {
	case speed <= 0:
		// No sample yet (just started) — not a stall, just unknown. Report OK-with-caveat rather
		// than alarming on a channel that has simply not produced its first progress line.
		return HealthOK, "Transcoding; no speed sample yet (warming up)."
	case speed < stallSpeed:
		where := "software (CPU)"
		if hardware {
			where = "hardware, but starved (check GPU/VRAM below)"
		}
		return HealthStalled, fmt.Sprintf("Transcoding at %.2f× realtime on %s — below 1.0×, the channel will buffer.", speed, where)
	case speed < degradedSpeed:
		return HealthDegraded, fmt.Sprintf("Transcoding at %.2f× realtime — playing, but little margin.", speed)
	default:
		return HealthOK, fmt.Sprintf("Transcoding at %.2f× realtime — comfortable margin.", speed)
	}
}

// statusGPU reads the GPU + resident-LLM header from the SystemLLM status (which already probes
// nvidia-smi + Ollama /api/ps for the model picker, §8.1). Best-effort: any failure → empty header.
func (s *Server) statusGPU(ctx context.Context) PlayoutGPU {
	if s.systemLLM == nil {
		return PlayoutGPU{}
	}
	status, err := s.systemLLM.Status(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Debug("playout status: LLM/GPU status unavailable", "err", err)
		}
		return PlayoutGPU{}
	}
	g := PlayoutGPU{Name: status.GPUName, VRAMGiB: status.VRAMGiB}

	// ⚠ **LIVE residency, from Ollama /api/ps — not the on-disk-size estimate this used to report.**
	// The old path looked up the active model tag in the catalog and reported its on-DISK size as
	// "VRAM held". That lied in the exact case this status exists to diagnose: it showed a model as
	// resident and holding several GB even after Ollama had unloaded it — so `contended` fired (or
	// didn't) on a fiction, and the one tool meant to confirm GPU contention could not be trusted.
	// residentLLMVRAM asks Ollama what is ACTUALLY loaded right now; nothing resident → 0 held → no
	// contention, a real answer. Best-effort: nil injection or a probe error leaves the header at 0,
	// never a failed request (the channel-health rows do not depend on it).
	if s.residentLLMVRAM != nil {
		if gib, model := s.residentLLMVRAM(ctx); gib > 0 {
			g.LLMVRAMGiB, g.LLMModel = gib, model
		}
	}
	return g
}
