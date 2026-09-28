import { type Density, Screen, Surface, Text } from "@loomarr/design-system";
import { type ClientDestination, ClientNavigation, clientDestinationLabel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const NavigationWorkshop = ({
  density = "pointer",
  variant,
}: {
  density?: Density;
  variant?: "material";
}) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  const navigation = (
    <ClientNavigation active={active} density={density} onNavigate={setActive} variant={variant} />
  );
  // A phone docks its tab bar under the content, as ClientShell does.
  const phone = density === "touch";
  return (
    <Screen density={density} footer={phone ? navigation : undefined} gap="$section">
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
      {phone ? null : navigation}
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
// iPhone's tab bar (#1659 native mock 5d, 5e): the web build of a touch surface.
const Touch: Story = { args: { density: "touch" } };
// Android's Material bar with the pill (5f, 5g).
const Material: Story = { args: { density: "touch", variant: "material" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Surf" }).focus();
  },
};
const TouchFocused: Story = {
  args: { density: "touch" },
  play: async ({ canvas }) => {
    canvas.getByRole("tab", { name: "Surf" }).focus();
  },
};

export default meta;
export { Focused, Light, Material, Pointer, Touch, TouchFocused, Tv };
