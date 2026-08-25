import { Screen, Surface, Text } from "@loomarr/design-system";
import { ModalOverlay, TransientOverlay } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const NativeOverlayWorkshop = ({
  density = "touch",
  kind = "transient",
}: {
  density?: "touch" | "tv";
  kind?: "modal" | "transient";
}) => {
  const [visible, setVisible] = useState(true);
  return (
    <Screen density={density}>
      <Surface flex={1} justifyContent="center" padding="$section">
        <Text density={density} textRole="title">
          Playback remains mounted
        </Text>
      </Surface>
      {kind === "modal" ? (
        <ModalOverlay
          actions={[
            { label: "Keep watching", onPress: () => setVisible(false), preferredFocus: true },
            { label: "Leave playback", onPress: () => setVisible(false), tone: "danger" },
          ]}
          density={density}
          description="Your tuned channel will stop playing on this device."
          onDismiss={() => setVisible(false)}
          title="Return to the device home screen?"
          visible={visible}
        />
      ) : (
        <TransientOverlay
          density={density}
          description="The Simpsons · Marge vs. the Monorail · 7:00–7:30 PM"
          eyebrow="07 · CLASSIC ANIMATION"
          onDismiss={() => setVisible(false)}
          title="Now playing"
          visible={visible}
        />
      )}
    </Screen>
  );
};

const meta = {
  title: "Loomarr/Overlay",
  component: NativeOverlayWorkshop,
  args: { density: "touch", kind: "transient" },
} satisfies Meta<typeof NativeOverlayWorkshop>;

type Story = StoryObj<typeof meta>;
const TouchTransient: Story = {};
const TvTransient: Story = { args: { density: "tv" } };
const TouchModal: Story = { args: { kind: "modal" } };
const TvModal: Story = { args: { density: "tv", kind: "modal" } };
const LightModal: Story = { args: { kind: "modal" }, globals: { theme: "light" } };

export default meta;
export { LightModal, TouchModal, TouchTransient, TvModal, TvTransient };
