import { Action, type Density, Screen, Surface, Text } from "@loomarr/design-system";
import { ModalOverlay, TransientOverlay } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

type OverlayWorkshopProps = {
  autoDismissMs?: number;
  density?: Density;
  initialVisible?: boolean;
  kind?: "modal" | "transient";
  placement?: "bottom" | "top";
  reducedMotion?: boolean;
};

const OverlayWorkshop = ({
  autoDismissMs,
  density = "pointer",
  initialVisible = true,
  kind = "transient",
  placement = "bottom",
  reducedMotion,
}: OverlayWorkshopProps) => {
  const [visible, setVisible] = useState(initialVisible);
  return (
    <Screen density={density} gap="$section">
      <Surface
        backgroundColor="$surfaceElevated"
        flex={1}
        gap="$section"
        justifyContent="center"
        padding="$section"
      >
        <Text density={density} textRole="metadata" tone="live">
          PLAYBACK REMAINS MOUNTED
        </Text>
        <Text density={density} textRole="display">
          Classic Animation
        </Text>
        <Text density={density} textRole="body">
          The overlay composes over this surface without replacing the tuned channel.
        </Text>
        <Action density={density} onPress={() => setVisible(true)} tone="secondary">
          Open overlay
        </Action>
      </Surface>

      {kind === "modal" ? (
        <ModalOverlay
          actions={[
            {
              label: "Keep watching",
              onPress: () => setVisible(false),
              preferredFocus: true,
              tone: "secondary",
            },
            { label: "Leave playback", onPress: () => setVisible(false), tone: "danger" },
          ]}
          density={density}
          description="Your tuned channel will stop playing on this device."
          eyebrow="LEAVE LOOMARR"
          onDismiss={() => setVisible(false)}
          reducedMotion={reducedMotion}
          title="Return to the device home screen?"
          visible={visible}
        />
      ) : (
        <TransientOverlay
          actions={[{ label: "View Guide", onPress: () => undefined, tone: "secondary" }]}
          autoDismissMs={autoDismissMs}
          density={density}
          description="The Simpsons · Marge vs. the Monorail · 7:00–7:30 PM"
          eyebrow="07 · CLASSIC ANIMATION"
          onDismiss={() => setVisible(false)}
          placement={placement}
          reducedMotion={reducedMotion}
          title="Now playing"
          visible={visible}
        />
      )}
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Overlay",
  component: OverlayWorkshop,
  args: { density: "pointer", initialVisible: true, kind: "transient", placement: "bottom" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof OverlayWorkshop>;

type Story = StoryObj<typeof meta>;
const TransientBottom: Story = {};
const TransientTop: Story = { args: { placement: "top" } };
const TouchTransient: Story = { args: { density: "touch" } };
const TvTransient: Story = { args: { density: "tv" } };
const LightTransient: Story = { globals: { theme: "light" } };
const Confirmation: Story = { args: { kind: "modal" }, tags: ["portal"] };
const LightConfirmation: Story = {
  args: { kind: "modal" },
  globals: { theme: "light" },
  tags: ["portal"],
};
const TvConfirmation: Story = { args: { density: "tv", kind: "modal" }, tags: ["portal"] };
const ReducedMotionConfirmation: Story = {
  args: { kind: "modal", reducedMotion: true },
  tags: ["portal"],
};
const InteractiveConfirmation: Story = {
  args: { initialVisible: false, kind: "modal", reducedMotion: true },
  tags: ["motion-only", "portal"],
};
const AutoDismiss: Story = {
  args: { autoDismissMs: 500 },
  tags: ["motion-only"],
};

export default meta;
export {
  AutoDismiss,
  Confirmation,
  InteractiveConfirmation,
  LightConfirmation,
  LightTransient,
  ReducedMotionConfirmation,
  TouchTransient,
  TransientBottom,
  TransientTop,
  TvConfirmation,
  TvTransient,
};
