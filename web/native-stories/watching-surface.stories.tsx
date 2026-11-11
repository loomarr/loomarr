import { LoomarrProvider } from "@loomarr/design-system";
import { WatchingSurface, type WatchingSurfaceProps } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

const channel = { id: "nature", inAppPlayable: true, name: "Nature Documentaries", number: 21 };
const games = { id: "games", inAppPlayable: true, name: "Game Shows", number: 12 };
type WatchingSnapshot = WatchingSurfaceProps["snapshot"];

// The live edge, on a whole minute so the clocks read the same in every run.
const NOW = Date.UTC(2026, 10, 11, 2, 25);
const MIN = 60_000;

const snapshot: WatchingSnapshot = {
  attemptId: 3,
  catalog: [games, channel],
  channel,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: NOW },
  previousChannelId: "games",
  recentChannelIds: ["games"],
  status: "playing",
};

const chrome: NonNullable<WatchingSurfaceProps["chrome"]> = {
  band: {
    channelName: "Nature Documentaries",
    channelNumber: "21",
    clockMs: NOW,
    decade: 1990,
    line: "S2 E4 “The Deep”",
    next: { name: "A Year on the Reef", startMs: NOW + 8 * MIN },
    progress: 25 / 30,
    rating: "TV-PG",
    startMs: NOW - 25 * MIN,
    stopMs: NOW + 5 * MIN,
    title: "Ocean Worlds",
  },
  segments: [
    {
      chips: ["S2 E4", "TV-PG", "Nature"],
      id: "now",
      kind: "programme",
      line: "S2 E4 “The Deep”",
      name: "Ocean Worlds",
      startMs: NOW - 25 * MIN,
      stopMs: NOW + 5 * MIN,
      synopsis: "Down past the light, where the water is cold and the animals make their own.",
    },
    { id: "break", kind: "break", spots: [], startMs: NOW + 5 * MIN, stopMs: NOW + 8 * MIN },
    {
      chips: ["1996", "TV-G", "Nature"],
      id: "next",
      kind: "programme",
      name: "A Year on the Reef",
      startMs: NOW + 8 * MIN,
      stopMs: NOW + 68 * MIN,
    },
  ],
  window: { endMs: NOW + 90 * MIN, nowMs: NOW, startMs: NOW - 15 * MIN },
};

/** A data: URI, never a remote URL: the snapshot suite must not depend on the network. */
const STILL =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAgAAAAECAIAAAA8r+mnAAAAVUlEQVR4nA3JIRIAIQwDwHsTGh1dHY2OruxUIZFIZCVPPNbu1+AdAUzDIvbAEcpxv0bvDHAaF7kHj1jOF/KugKZpUXvoSOV6kd4zkNNyMffIoyzP+wOonSmh7IzuzQAAAABJRU5ErkJggg==";

// The band's clocks, in UTC so the render does not depend on the machine's zone.
const formatTime = (ms: number) => {
  const date = new Date(ms);
  const hours = date.getUTCHours();
  return `${hours % 12 || 12}:${String(date.getUTCMinutes()).padStart(2, "0")} ${hours < 12 ? "AM" : "PM"}`;
};

const Preview = ({
  chromeData = chrome,
  controlsVisible = true,
  loading,
  loadError,
  numberEntry,
  state = snapshot,
}: {
  chromeData?: WatchingSurfaceProps["chrome"];
  controlsVisible?: boolean;
  loading?: boolean;
  loadError?: string;
  numberEntry?: WatchingSurfaceProps["numberEntry"];
  state?: WatchingSnapshot;
}) => (
  <LoomarrProvider>
    <View style={{ height: "100%", width: "100%" }}>
      <WatchingSurface
        chrome={chromeData}
        controlsVisible={controlsVisible}
        formatTime={formatTime}
        loading={loading}
        loadError={loadError}
        numberEntry={numberEntry}
        onChangeServer={() => undefined}
        onDismissControls={() => undefined}
        onGoLive={() => undefined}
        onOpenGuide={() => undefined}
        onRetry={() => undefined}
        onTuneChannel={() => undefined}
        playheadMs={state.livePlayback?.viewerTimeMs ?? NOW}
        player={<View style={{ backgroundColor: "#1B1E24", flex: 1 }} />}
        snapshot={state}
      />
    </View>
  </LoomarrProvider>
);

const meta = {
  component: Preview,
  title: "Loomarr Components/Watching Surface",
} satisfies Meta<typeof Preview>;

type Story = StoryObj<typeof meta>;
const CurrentAndNext: Story = {};
const ControlsHidden: Story = { args: { controlsVisible: false } };
const NothingScheduled: Story = {
  args: {
    chromeData: {
      band: { channelName: "Nature Documentaries", channelNumber: "21", clockMs: NOW },
      segments: [],
      window: chrome.window,
    },
  },
};
const Loading: Story = {
  args: {
    chromeData: undefined,
    loading: true,
    state: { catalog: [], recentChannelIds: [], status: "empty" },
  },
};
const Tuning: Story = { args: { state: { ...snapshot, status: "tuning" } } };
// A viewer-initiated tune (channel up/down) is what raises the switch overlay: the surf card over the
// channel's dimmed still, or over plain black when the channel has no still yet.
const TuningSwitch: Story = { args: { state: { ...snapshot, status: "tuning", tuneReason: "step" } } };
const TuningSwitchWithStill: Story = {
  args: { state: { ...snapshot, status: "tuning", stillUri: STILL, tuneReason: "step" } },
};
const PlaybackError: Story = {
  args: { state: { ...snapshot, error: "The stream could not be decoded.", status: "failed" } },
};
const LoadError: Story = {
  args: {
    chromeData: undefined,
    loadError: "Loomarr didn't answer. Check that the server is running.",
    state: { catalog: [], recentChannelIds: [], status: "empty" },
  },
};
const NumberEntry: Story = {
  // No expiry: a countdown would tick between renders. The unit tests read it.
  args: { numberEntry: { channelName: "Nature Documentaries", digits: "21" } },
};
const Paused: Story = {
  args: {
    chromeData: { ...chrome, band: { ...chrome.band, behindLabel: "0:23 behind", liveProgress: 25 / 30 } },
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 23, mode: "paused", noticeRevision: 0, viewerTimeMs: NOW - 23_000 },
      status: "paused",
    },
  },
};
const BehindLive: Story = {
  args: {
    chromeData: { ...chrome, band: { ...chrome.band, behindLabel: "1:23 behind", liveProgress: 25 / 30 } },
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 83, mode: "behind", noticeRevision: 0, viewerTimeMs: NOW - 83_000 },
    },
  },
};
const EmptyChannel: Story = {
  args: { chromeData: undefined, state: { catalog: [], recentChannelIds: [], status: "empty" } },
};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export {
  BehindLive,
  ControlsHidden,
  CurrentAndNext,
  EmptyChannel,
  Light,
  LoadError,
  Loading,
  NothingScheduled,
  NumberEntry,
  Paused,
  PlaybackError,
  Tuning,
  TuningSwitch,
  TuningSwitchWithStill,
};
