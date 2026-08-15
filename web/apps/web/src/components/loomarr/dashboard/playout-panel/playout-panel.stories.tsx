import type { PlayoutStatus } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlayoutPanel } from "./playout-panel";

const meta: Meta<typeof PlayoutPanel> = {
  title: "Dashboard/PlayoutPanel",
  component: PlayoutPanel,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PlayoutPanel>;

const status = (over: Partial<PlayoutStatus>): PlayoutStatus =>
  ({
    running: true,
    gpu: { name: "NVIDIA GeForce RTX 3080 Ti", vramGiB: 12, contended: false },
    prepared: {
      available: true,
      running: false,
      lastRunAt: "2026-08-14T12:00:00Z",
      channels: 100,
      readyChannels: 100,
      scheduledBindings: 320,
      readyBindings: 320,
      missingBindings: 0,
      queuedPublications: 0,
      remainingBytes: 180 * 1024 ** 3,
      budgetBytes: 512 * 1024 ** 3,
      protectedBytes: 140 * 1024 ** 3,
    },
    channels: [],
    ...over,
  }) as PlayoutStatus;

// A healthy hardware channel: human label, throughput + fast cold-start, OK verdict.
export const Healthy: Story = {
  args: {
    status: status({
      channels: [
        {
          channelId: "ch_9bd1fd7609b5f925",
          number: 3,
          name: "1980s Action Heroes",
          target: "browser",
          viewers: 2,
          bufferedMs: 8_000,
          encoder: "h264_nvenc",
          hardware: true,
          mode: "transcode",
          speed: 8.4,
          health: "ok",
          reason: "Transcoding at 8.4× realtime — comfortable margin.",
          coldStartMs: 400,
        },
      ],
    }),
  },
};

// The case the panel exists for: an LLM resident on the encoders' GPU, a channel one bad beat from
// stalling. The contention badge + the DEGRADED verdict tell the operator why.
export const Contended: Story = {
  args: {
    status: status({
      gpu: {
        name: "NVIDIA GeForce RTX 3080 Ti",
        vramGiB: 12,
        llmModel: "qwen3:8b",
        llmVramGiB: 5.6,
        contended: true,
      },
      channels: [
        {
          channelId: "ch_2986070a483d5cb0",
          number: 1,
          name: "Springfield Classics",
          target: "browser",
          viewers: 1,
          bufferedMs: -2_000,
          encoder: "h264_nvenc",
          hardware: true,
          mode: "transcode",
          speed: 1.14,
          health: "degraded",
          reason: "Transcoding at 1.14× realtime — playing, but little margin.",
          coldStartMs: 1_300,
        },
      ],
    }),
  },
};

// Running, but nobody is watching — distinct from "unhealthy".
export const Idle: Story = { args: { status: status({ channels: [] }) } };

// A Tunarr-backed install: Loomarr is not the one streaming, which is not the same as a fault.
export const TunarrBacked: Story = {
  args: { status: status({ running: false, gpu: { vramGiB: 0, contended: false }, channels: [] }) },
};

// Software-only box: no GPU header, channels still play.
export const SoftwareOnly: Story = {
  args: {
    status: status({
      gpu: { vramGiB: 0, contended: false },
      channels: [
        {
          channelId: "ch_f4bcb4d63efeffec",
          number: 7,
          name: "Late-Night Sci-Fi",
          target: "browser",
          viewers: 1,
          bufferedMs: 3_000,
          encoder: "libx264",
          hardware: false,
          mode: "transcode",
          speed: 1.6,
          health: "ok",
          reason: "Transcoding at 1.6× realtime on software (CPU).",
          coldStartMs: 2_100,
        },
      ],
    }),
  },
};

export const Loading: Story = { args: { loading: true } };
