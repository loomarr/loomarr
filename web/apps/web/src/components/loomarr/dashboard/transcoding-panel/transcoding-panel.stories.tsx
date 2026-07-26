import type { PlayoutTelemetry } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { TranscodingPanel } from "./transcoding-panel";

// Live internal-playout telemetry. Every number is MEASURED — ffmpeg's structured progress was
// parsed and discarded before V16 wired the callback through.
const meta = {
  title: "Dashboard/TranscodingPanel",
  component: TranscodingPanel,
  decorators: [widthFrame(620)],
} satisfies Meta<typeof TranscodingPanel>;

type Story = StoryObj<typeof meta>;

const Streaming: Story = {
  args: {
    telemetry: {
      running: true,
      active: 2,
      capacity: 4,
      sessions: [
        {
          channelId: "ch-42",
          viewers: 2,
          encoder: "h264_nvenc",
          hardware: true,
          speed: 12.4,
          bufferedMs: 96_000,
          uptimeMs: 720_000,
        },
        {
          channelId: "ch-7",
          viewers: 1,
          encoder: "libx264",
          hardware: false,
          speed: 1.4,
          bufferedMs: 12_000,
          uptimeMs: 180_000,
        },
      ],
    } as PlayoutTelemetry,
  },
};

// A software encoder losing to realtime: sustained under 1.0 means the channel will stutter,
// and the deficit shows as "behind" rather than a buffer.
const FallingBehind: Story = {
  args: {
    telemetry: {
      running: true,
      active: 4,
      capacity: 4,
      sessions: [
        {
          channelId: "ch-3",
          viewers: 1,
          encoder: "libx264",
          hardware: false,
          speed: 0.8,
          bufferedMs: -4_000,
          uptimeMs: 300_000,
        },
      ],
    } as PlayoutTelemetry,
  },
};

// Playout running, nobody watching. A channel starts encoding when someone tunes in.
const Idle: Story = {
  args: { telemetry: { running: true, active: 0, capacity: 4, sessions: [] } as PlayoutTelemetry },
};

// ⚠ NOT the same as idle. On a Tunarr-backed install the list is legitimately empty, and a
// panel that cannot tell the two apart reads as every channel having just died.
const TunarrBacked: Story = {
  args: { telemetry: { running: false, active: 0, capacity: 0, sessions: [] } as PlayoutTelemetry },
};

export default meta;
export { FallingBehind, Idle, Streaming, TunarrBacked };
