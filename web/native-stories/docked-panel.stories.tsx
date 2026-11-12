import { DockedPanel, Screen, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

// The docked panel sits inside a frame that is not the screen: here a stand-in for the player's
// picture. It has no scrim and takes no focus, so the picture stays visible and live beside it.
const NativeDockedPanelWorkshop = ({
  density = "touch",
  edge = "right",
}: {
  density?: "touch" | "tv";
  edge?: "left" | "right";
}) => (
  <Screen density={density}>
    <View style={{ backgroundColor: "#202833", flex: 1, justifyContent: "center", padding: 24 }}>
      <Text density={density} textRole="title">
        Playback keeps running
      </Text>
      <DockedPanel
        closeLabel="Close channels"
        density={density}
        edge={edge}
        meta="now to next 2h"
        onClose={() => undefined}
        title="Channels"
      >
        <View style={{ gap: 8, padding: 10 }}>
          <Text density={density} textRole="label" tone="secondary">
            07 · Westerns
          </Text>
          <Text density={density} textRole="label" tone="secondary">
            08 · Cartoons
          </Text>
        </View>
      </DockedPanel>
    </View>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Docked Panel",
  component: NativeDockedPanelWorkshop,
  args: { density: "touch", edge: "right" },
} satisfies Meta<typeof NativeDockedPanelWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const TouchLeft: Story = { args: { edge: "left" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, TouchLeft, Tv };
