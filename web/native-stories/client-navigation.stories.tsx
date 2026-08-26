import { Screen, Surface, Text } from "@loomarr/design-system";
import { type ClientDestination, ClientNavigation, clientDestinationLabel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const NativeNavigationWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [active, setActive] = useState<ClientDestination>(density === "tv" ? "watching" : "guide");
  return (
    <Screen density={density} gap="$section">
      <Surface flex={1} justifyContent="center" padding="$section">
        <Text density={density} textRole="title">
          {clientDestinationLabel(active)}
        </Text>
      </Surface>
      <ClientNavigation active={active} density={density} onNavigate={setActive} />
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Client Navigation",
  component: NativeNavigationWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeNavigationWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
