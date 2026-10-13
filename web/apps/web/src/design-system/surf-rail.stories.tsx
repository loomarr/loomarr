import { Screen, Surface, Text } from "@loomarr/design-system";
import { surfGroups } from "@loomarr/fixtures";
import { SurfRail, type SurfSelection } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const Artwork = () => (
  <Surface
    alignItems="center"
    backgroundColor="$stateInfoSurface"
    borderWidth={0}
    flex={1}
    justifyContent="center"
    width="100%"
  >
    <Text textRole="label" tone="primary">
      KETTLE KIDS
    </Text>
  </Surface>
);

const SurfWorkshop = ({
  density = "pointer",
  railActions = false,
  serverVersion = "0.2.1",
}: {
  density?: "pointer" | "touch" | "tv";
  /** The TV rail's channel-number wait and Disconnect rows under the list. */
  railActions?: boolean;
  serverVersion?: string;
}) => {
  const [selection, setSelection] = useState<SurfSelection>({
    channelId: "ch-cartoons",
    group: "recent",
  });
  const [tuned, setTuned] = useState("ch-cartoons");
  const [autoTuneMs, setAutoTuneMs] = useState(1_200);
  return (
    <Screen density={density} position="relative">
      <Surface alignItems="center" flex={1} justifyContent="center">
        <Text density={density} textRole="metadata" tone="muted">
          PLAYBACK REMAINS MOUNTED · {tuned}
        </Text>
      </Surface>
      <Surface
        backgroundColor="$transparent"
        borderWidth={0}
        bottom={0}
        justifyContent="center"
        left={0}
        position="absolute"
        right={0}
        top={0}
      >
        <SurfRail
          autoTune={
            railActions
              ? { choicesMs: [1_200, 2_000, 5_000], onChange: setAutoTuneMs, valueMs: autoTuneMs }
              : undefined
          }
          clientVersion="0.2.0"
          density={density}
          groups={surfGroups}
          onDisconnect={railActions ? () => undefined : undefined}
          onFocusSelection={setSelection}
          onTune={setTuned}
          renderArtwork={(channel) => (channel.id === "ch-cartoons" ? <Artwork /> : undefined)}
          selection={selection}
          serverVersion={serverVersion || undefined}
        />
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Surf Rail",
  component: SurfWorkshop,
  args: { density: "pointer", serverVersion: "0.2.1" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SurfWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const ServerUnavailable: Story = { args: { serverVersion: "" } };
const FocusedAllChannel: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "All channels, channel 2, Sci-Fi Classics" }).focus();
  },
};

// #1887: focus on a rail action leaves the selected channel expanded but at rest, so the focused
// row is the only amber on screen.
const TvRailActionFocused: Story = {
  args: { density: "tv", railActions: true },
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Channel-number wait · 1.2 s" }).focus();
  },
};

export default meta;
export { FocusedAllChannel, Light, Pointer, ServerUnavailable, Touch, Tv, TvRailActionFocused };
