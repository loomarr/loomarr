import type { PlayoutDoctor } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PlayoutHealthPanel } from "./playout-health-panel";

// "Is playout actually healthy" — GPU/VRAM contention plus a per-channel diagnosis. The
// contention story is the one this panel exists for: an LLM kept warm for suggest latency
// (§8.2) silently eating the VRAM the encoders need.
const meta = {
  title: "Dashboard/PlayoutHealthPanel",
  component: PlayoutHealthPanel,
  decorators: [widthFrame(620)],
} satisfies Meta<typeof PlayoutHealthPanel>;

type Story = StoryObj<typeof meta>;

const Healthy: Story = {
  args: {
    doctor: {
      running: true,
      gpu: {
        name: "NVIDIA GeForce RTX 3080 Ti",
        vramGiB: 12,
        llmModel: "qwen3:8b",
        llmVramGiB: 4.9,
        contended: false,
      },
      channels: [
        {
          channelId: "ch-42",
          target: "browser",
          encoder: "h264_vulkan",
          hardware: true,
          mode: "transcode",
          speed: 3.2,
          health: "ok",
          reason: "Encoding comfortably ahead of realtime.",
        },
        {
          channelId: "ch-7",
          target: "mediaserver",
          encoder: "copy",
          hardware: false,
          mode: "direct-play",
          speed: 0,
          health: "ok",
          reason: "Direct playing — no encode needed.",
        },
      ],
    } as PlayoutDoctor,
  },
};

// The panel's reason for existing: a resident LLM sharing the GPU with an active hardware
// encode is a real, silent cause of stutter that nothing else on the dashboard reports.
const Contended: Story = {
  args: {
    doctor: {
      running: true,
      gpu: {
        name: "NVIDIA GeForce RTX 3080 Ti",
        vramGiB: 12,
        llmModel: "qwen3:8b",
        llmVramGiB: 4.9,
        contended: true,
      },
      channels: [
        {
          channelId: "ch-42",
          target: "browser",
          encoder: "h264_vulkan",
          hardware: true,
          mode: "transcode",
          speed: 0.9,
          health: "degraded",
          reason: "VRAM pressure from a resident LLM is slowing the encoder.",
        },
      ],
    } as PlayoutDoctor,
  },
};

// A channel that has fallen behind hard enough to stutter for viewers, not just run close to
// the line — the strongest tone the health chip carries.
const Stalled: Story = {
  args: {
    doctor: {
      running: true,
      gpu: { vramGiB: 0, contended: false },
      channels: [
        {
          channelId: "ch-3",
          target: "browser",
          encoder: "libx264",
          hardware: false,
          mode: "transcode",
          speed: 0.7,
          health: "stalled",
          reason: "Software encoder can't keep up on this box.",
        },
      ],
    } as PlayoutDoctor,
  },
};

// No GPU detected at all — every channel that transcodes is on the CPU.
const SoftwareOnly: Story = {
  args: {
    doctor: {
      running: true,
      gpu: { vramGiB: 0, contended: false },
      channels: [
        {
          channelId: "ch-9",
          target: "browser",
          encoder: "libx264",
          hardware: false,
          mode: "transcode",
          speed: 1.8,
          health: "ok",
          reason: "Software encoding, keeping up fine.",
        },
      ],
    } as PlayoutDoctor,
  },
};

// ⚠ NOT the same as "no channels". On a Tunarr-backed install there is no internal playout to
// diagnose at all, which the backend sends explicitly rather than leaving the panel to guess
// from an empty list.
const TunarrBacked: Story = {
  args: {
    doctor: { running: false, gpu: { vramGiB: 0, contended: false }, channels: [] } as PlayoutDoctor,
  },
};

const Empty: Story = {
  args: {
    doctor: {
      running: true,
      gpu: { name: "NVIDIA GeForce RTX 3080 Ti", vramGiB: 12, contended: false },
      channels: [],
    } as PlayoutDoctor,
  },
};

export default meta;
export { Contended, Empty, Healthy, SoftwareOnly, Stalled, TunarrBacked };
