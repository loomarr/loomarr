package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/playout"
)

// The playout doctor (§9.1 V47) — one endpoint that answers "why is this channel black?".
//
// Several distinct failures all present to a viewer as a black frame: a codec the target can't
// decode, a hardware encode starved of VRAM by the resident LLM (§8.2), a transcode running below
// realtime, a channel with no session at all. Telling them apart means correlating the live encoders
// (speed is the tell — a sustained sub-1.0× is a stall), the GPU + its VRAM, and the resident LLM
// sharing that VRAM. This composes exactly those into one read-only health picture. It is the in-app
// twin of scripts/playout-diag.sh, and it changes nothing — safe to poll, safe to hand a support
// request.

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
type ChannelHealth struct {
	ChannelID string        `json:"channelId"`
	Target    string        `json:"target" doc:"Codec audience: browser|mediaserver (§9.1 V47)"`
	Encoder   string        `json:"encoder" doc:"Resolved encoder, or 'copy' when direct-playing"`
	Hardware  bool          `json:"hardware" doc:"Whether the encoder is hardware-accelerated"`
	Mode      string        `json:"mode" doc:"direct-play (copy) | transcode"`
	Speed     float64       `json:"speed" doc:"Realtime multiple; <1.0 means the channel will stutter"`
	Health    PlayoutHealth `json:"health" doc:"ok | degraded | stalled"`
	Reason    string        `json:"reason" doc:"One line an operator can act on"`
}

// DoctorGPU is the shared-GPU header — the contention picture the encoder rows alone can't show.
type DoctorGPU struct {
	Name       string  `json:"name,omitempty"`
	VRAMGiB    float64 `json:"vramGiB,omitempty" doc:"Total detected GPU VRAM (0 = unknown/none)"`
	LLMModel   string  `json:"llmModel,omitempty" doc:"Resident LLM sharing this GPU, if any (§8.2)"`
	LLMVRAMGiB float64 `json:"llmVramGiB,omitempty" doc:"VRAM the resident LLM holds — unavailable to encoders"`
	Contended  bool    `json:"contended" doc:"A model is resident AND channels are transcoding on hardware"`
}

// PlayoutDoctor is the whole health picture.
type PlayoutDoctor struct {
	Running  bool            `json:"running" doc:"Internal playout is wired (false on a Tunarr-only install)"`
	GPU      DoctorGPU       `json:"gpu"`
	Channels []ChannelHealth `json:"channels"`
}

type playoutDoctorOutput struct {
	Body PlayoutDoctor
}

// registerPlayoutDoctor mounts GET /v1/playout/doctor (§9.1 V47).
func (s *Server) registerPlayoutDoctor(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-playout-doctor", Method: http.MethodGet, Path: "/v1/playout/doctor",
		Summary: "Playout health — why a channel is (or isn't) playing (admin)",
		Description: "A read-only health projection of live internal playout: per (channel, target) " +
			"encoder + realtime-speed verdict (ok|degraded|stalled), plus the GPU/VRAM and resident-LLM " +
			"contention picture that a black-frame diagnosis needs. Admin only. Changes nothing.",
		Tags: []string{"dashboard"},
	}, RoleAdmin), s.getPlayoutDoctor)
}

func (s *Server) getPlayoutDoctor(ctx context.Context, _ *struct{}) (*playoutDoctorOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	return &playoutDoctorOutput{Body: s.playoutDoctor(ctx, time.Now())}, nil
}

// playoutDoctor builds the health picture from live state. Best-effort on the GPU/LLM header — a
// probe that fails degrades to an empty header, never a failed request, because the channel health
// (the load-bearing part) does not depend on it.
func (s *Server) playoutDoctor(ctx context.Context, now time.Time) PlayoutDoctor {
	if s.playoutSessions == nil {
		return PlayoutDoctor{Channels: []ChannelHealth{}} // Tunarr-only: not our job
	}

	channels := make([]ChannelHealth, 0)
	hwTranscoding := false
	for _, st := range s.playoutSessions.Stats(now) {
		h := channelHealthFrom(st)
		if h.Mode == "transcode" && st.Hardware {
			hwTranscoding = true
		}
		channels = append(channels, h)
	}

	gpu := s.doctorGPU(ctx)
	// Contention is the specific bad combination: a model is holding VRAM AND channels are
	// transcoding on the GPU. That is the state that starved the encoders and blacked channels
	// (§8.2) — worth flagging distinctly from "a model happens to be loaded" (harmless when idle).
	gpu.Contended = gpu.LLMVRAMGiB > 0 && hwTranscoding

	return PlayoutDoctor{Running: true, GPU: gpu, Channels: channels}
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
		Encoder: st.Encoder, Hardware: st.Hardware,
		Mode: mode, Speed: st.Speed, Health: health, Reason: reason,
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

// doctorGPU reads the GPU + resident-LLM header from the SystemLLM status (which already probes
// nvidia-smi + Ollama /api/ps for the model picker, §8.1). Best-effort: any failure → empty header.
func (s *Server) doctorGPU(ctx context.Context) DoctorGPU {
	if s.systemLLM == nil {
		return DoctorGPU{}
	}
	status, err := s.systemLLM.Status(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Debug("playout doctor: LLM/GPU status unavailable", "err", err)
		}
		return DoctorGPU{}
	}
	g := DoctorGPU{Name: status.GPUName, VRAMGiB: status.VRAMGiB}
	// The active model + its footprint: Status.Model is the active tag; find it in the catalog for
	// its VRAM proxy (the on-disk size). Only for a LOCAL provider — a hosted model holds no local
	// VRAM. This is a footprint ESTIMATE (on-disk size), not live residency; the exact resident set
	// is what scripts/playout-diag.sh reads from Ollama /api/ps, and the contention flag below only
	// needs "is a local model configured and sized", which this answers.
	if status.Local && status.Model != "" {
		for _, m := range status.Catalog {
			if m.Tag == status.Model && m.VRAMGiB > 0 {
				g.LLMModel, g.LLMVRAMGiB = m.Tag, m.VRAMGiB
				break
			}
		}
	}
	return g
}
