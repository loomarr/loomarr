import { LoomarrProvider } from "@loomarr/design-system";
import { type WatchingScheduleData, WatchingSurface, type WatchingSurfaceProps } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

const channel = { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 };
type WatchingSnapshot = WatchingSurfaceProps["snapshot"];

const snapshot: WatchingSnapshot = {
  attemptId: 3,
  catalog: [channel],
  channel,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 1_777_777_777_000 },
  previousChannelId: "six",
  recentChannelIds: ["six"],
  status: "playing",
};
const schedule: WatchingScheduleData = {
  next: { timeLabel: "9:30 PM", title: "The Next Frontier" },
  now: {
    badge: { label: "On now", tone: "live" },
    episodeLabel: "S1 E4",
    facts: ["2026", "TV-14", "Science fiction"],
    progressPercent: 42,
    timeLabel: "9:00 PM–9:30 PM",
    title: "The Current Frontier",
  },
};

const Preview = ({
  loading,
  loadError,
  numberEntry,
  scheduleData = schedule,
  state = snapshot,
}: {
  loading?: boolean;
  loadError?: string;
  numberEntry?: WatchingSurfaceProps["numberEntry"];
  scheduleData?: WatchingScheduleData;
  state?: WatchingSnapshot;
}) => (
  <LoomarrProvider>
    <View style={{ height: "100%", width: "100%" }}>
      <WatchingSurface
        density={process.env.EXPO_PUBLIC_LOOMARR_STORYBOOK_DENSITY === "tv" ? "tv" : "touch"}
        loading={loading}
        loadError={loadError}
        numberEntry={numberEntry}
        onChannelDown={() => undefined}
        onChannelUp={() => undefined}
        onDismissControls={() => undefined}
        onGoLive={() => undefined}
        onOpenGuide={() => undefined}
        onOpenSurf={() => undefined}
        onPause={() => undefined}
        onPlay={() => undefined}
        onPrevious={() => undefined}
        onRetry={() => undefined}
        onShowControls={() => undefined}
        player={<View style={{ backgroundColor: "#101316", flex: 1 }} />}
        schedule={scheduleData}
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
const Loading: Story = {
  args: { loading: true, scheduleData: {}, state: { catalog: [], recentChannelIds: [], status: "empty" } },
};
const Tuning: Story = { args: { state: { ...snapshot, status: "tuning" } } };
const PlaybackError: Story = {
  args: { state: { ...snapshot, error: "The stream could not be decoded.", status: "failed" } },
};
const NumberEntry: Story = {
  args: { numberEntry: { channelName: "Nature Documentaries", digits: "21" } },
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
  args: { scheduleData: {}, state: { catalog: [], recentChannelIds: [], status: "empty" } },
};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export {
  BehindLive,
  CurrentAndNext,
  EmptyChannel,
  Light,
  Loading,
  NumberEntry,
  Paused,
  PlaybackError,
  Tuning,
};
