import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { WatchingSurface } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

const snapshot: PlayerSnapshot = {
  attemptId: 3,
  catalog: [],
  channel: { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 },
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 1_777_777_777_000 },
  overlayVisible: true,
  previousChannelId: "six",
  recentChannelIds: ["six"],
  status: "playing",
};

const Preview = ({ state = snapshot }: { state?: PlayerSnapshot }) => (
  <LoomarrProvider>
    <View style={{ height: "100%", width: "100%" }}>
      <WatchingSurface
        density={process.env.EXPO_PUBLIC_LOOMARR_STORYBOOK_DENSITY === "tv" ? "tv" : "touch"}
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
        snapshot={state}
      />
    </View>
  </LoomarrProvider>
);

const meta = {
  component: Preview,
  title: "Loomarr Components/Watching Surface",
} satisfies Meta<typeof Preview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Playing: Story = {};
export const Paused: Story = {
  args: {
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 23, mode: "paused", noticeRevision: 0, viewerTimeMs: 1_777_777_754_000 },
      status: "paused",
    },
  },
};
export const BehindLive: Story = {
  args: {
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 83, mode: "behind", noticeRevision: 0, viewerTimeMs: 1_777_777_694_000 },
    },
  },
};
export const ExpiredPause: Story = {
  args: {
    state: {
      ...snapshot,
      livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 1, viewerTimeMs: 1_777_777_777_000 },
    },
  },
};
export const Tuning: Story = { args: { state: { ...snapshot, status: "tuning" } } };
export const Failed: Story = {
  args: { state: { ...snapshot, error: "The stream could not be decoded.", status: "failed" } },
};
export const Light: Story = { globals: { theme: "light" } };
