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
      SPRINGFIELD
    </Text>
  </Surface>
);

const SurfWorkshop = ({
  density = "pointer",
  serverVersion = "0.2.1",
}: {
  density?: "pointer" | "touch" | "tv";
  serverVersion?: string;
}) => {
  const [selection, setSelection] = useState<SurfSelection>({
    channelId: "ch-springfield",
    group: "recent",
  });
  const [tuned, setTuned] = useState("ch-springfield");
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
          clientVersion="0.2.0"
          density={density}
          groups={surfGroups}
          onFocusSelection={setSelection}
          onTune={setTuned}
          renderArtwork={(channel) => (channel.id === "ch-springfield" ? <Artwork /> : undefined)}
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
    canvas.getByRole("button", { name: "All channels, channel 2, Star Trek Classics" }).focus();
  },
};

export default meta;
export { FocusedAllChannel, Light, Pointer, ServerUnavailable, Touch, Tv };
