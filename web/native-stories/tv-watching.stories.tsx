import { LoomarrProvider } from "@loomarr/design-system";
import { TvWatching, type TvWatchingProps } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

const channel = { id: "nature", inAppPlayable: true, name: "Nature Documentaries", number: 21 };
type WatchingSnapshot = TvWatchingProps["snapshot"];

const snapshot: WatchingSnapshot = {
  attemptId: 3,
  catalog: [channel],
  channel,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 1_777_777_777_000 },
  previousChannelId: "games",
  recentChannelIds: ["games"],
  status: "playing",
};

/** A data: URI, never a remote URL: the snapshot suite must not depend on the network. */
const STILL =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAgAAAAECAIAAAA8r+mnAAAAVUlEQVR4nA3JIRIAIQwDwHsTGh1dHY2OruxUIZFIZCVPPNbu1+AdAUzDIvbAEcpxv0bvDHAaF7kHj1jOF/KugKZpUXvoSOV6kd4zkNNyMffIoyzP+wOonSmh7IzuzQAAAABJRU5ErkJggg==";

const noop = () => undefined;

const chromeFor = (state: WatchingSnapshot, ident = false): TvWatchingProps["chrome"] => {
  const live = state.livePlayback ?? { lagSeconds: 0, mode: "live" as const };
  return {
    band: {
      channelName: channel.name,
      channelNumber: String(channel.number),
      progress: 0.83,
      programmeLine: "Blue Planet · S2 E4 “The Deep”",
      station: "1990s · Portland, ME",
      timeLabel: "25m in · 5m left",
    },
    formatTime: () => "2:30 AM",
    ident,
    transport: {
      density: "tv",
      live,
      muted: false,
      onChannels: noop,
      onGoLive: noop,
      onGuide: noop,
      onSeekBack: noop,
      onSeekForward: noop,
      onToggleMute: noop,
      onTogglePlay: noop,
      onVolume: noop,
      playing: state.status === "playing",
      pointer: false,
      volume: 1,
    },
  };
};

const Preview = ({
  controlsShown = true,
  ident = false,
  loading = false,
  loadError,
  numberEntry,
  state = snapshot,
}: {
  controlsShown?: boolean;
  ident?: boolean;
  loading?: boolean;
  loadError?: string;
  numberEntry?: TvWatchingProps["numberEntry"];
  state?: WatchingSnapshot;
}) => (
  <LoomarrProvider>
    <View style={{ height: "100%", width: "100%" }}>
      <TvWatching
        chrome={state.channel ? chromeFor(state, ident) : undefined}
        chromeVisible
        controlsShown={controlsShown}
        loading={loading}
        loadError={loadError}
        numberEntry={numberEntry}
        onBarePress={noop}
        onChangeServer={noop}
        onLeave={noop}
        onRetry={noop}
        player={<View style={{ backgroundColor: "#1B1E24", flex: 1 }} />}
        snapshot={state}
      />
    </View>
  </LoomarrProvider>
);

const meta = {
  component: Preview,
  title: "Loomarr Components/TV Watching",
} satisfies Meta<typeof Preview>;

type Story = StoryObj<typeof meta>;
const Controls: Story = {};
const BarePicture: Story = { args: { controlsShown: false } };
const StationId: Story = { args: { controlsShown: false, ident: true } };
const Loading: Story = {
  args: { loading: true, state: { catalog: [], recentChannelIds: [], status: "empty" } },
};
// A viewer-initiated tune (channel up/down) is what raises the switch overlay: the surf card over the
// channel's dimmed still, or over plain black when the channel has no still yet.
const TuningSwitch: Story = { args: { state: { ...snapshot, status: "tuning", tuneReason: "step" } } };
const TuningSwitchWithStill: Story = {
  args: { state: { ...snapshot, status: "tuning", stillUri: STILL, tuneReason: "step" } },
};
const PlaybackError: Story = {
  args: { state: { ...snapshot, error: "The stream could not be decoded.", status: "failed" } },
};
const CatalogError: Story = {
  args: {
    loadError: "This Loomarr isn't answering.",
    state: { catalog: [], recentChannelIds: [], status: "empty" },
  },
};
const NumberEntry: Story = {
  args: { controlsShown: false, numberEntry: { channelName: "Nature Documentaries", digits: "21" } },
};
const Paused: Story = {
  args: {
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 23, mode: "paused", noticeRevision: 0, viewerTimeMs: 1_777_777_754_000 },
      status: "paused",
    },
  },
};
const BehindLive: Story = {
  args: {
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 83, mode: "behind", noticeRevision: 0, viewerTimeMs: 1_777_777_694_000 },
    },
  },
};
const EmptyChannel: Story = {
  args: { state: { catalog: [], recentChannelIds: [], status: "empty" } },
};
// The chrome stays dark over the picture in a light app (§9).
const Light: Story = { globals: { theme: "light" } };

export default meta;
export {
  BarePicture,
  BehindLive,
  CatalogError,
  Controls,
  EmptyChannel,
  Light,
  Loading,
  NumberEntry,
  Paused,
  PlaybackError,
  StationId,
  TuningSwitch,
  TuningSwitchWithStill,
};
