import { type Density, Screen, Surface, Text } from "@loomarr/design-system";
import { type ClientDestination, ClientNavigation, clientDestinationLabel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const NavigationWorkshop = ({ density = "pointer" }: { density?: Density }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  return (
    <Screen density={density} gap="$section">
      <Surface flex={1} gap="$control" justifyContent="center" padding="$section">
        <Text density={density} textRole="metadata" tone="info">
          CURRENT DESTINATION
        </Text>
        <Text density={density} textRole="display">
          {clientDestinationLabel(active)}
        </Text>
        <Text density={density} textRole="body">
          Watching remains the stable return point when Guide or Surf closes.
        </Text>
      </Surface>
      <ClientNavigation active={active} density={density} onNavigate={setActive} />
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Client Navigation",
  component: NavigationWorkshop,
  args: { density: "pointer" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof NavigationWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Surf" }).focus();
  },
};

export default meta;
export { Focused, Light, Pointer, Touch, Tv };
