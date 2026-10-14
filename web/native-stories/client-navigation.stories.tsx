import { Screen, Surface, Text } from "@loomarr/design-system";
import { type ClientDestination, ClientNavigation, clientDestinationLabel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const NativeNavigationWorkshop = ({
  density = "touch",
  variant,
}: {
  density?: "touch" | "tv";
  variant?: "material" | "tabBar";
}) => {
  const [active, setActive] = useState<ClientDestination>(density === "tv" ? "watching" : "guide");
  const navigation = (
    <ClientNavigation active={active} density={density} onNavigate={setActive} variant={variant} />
  );
  // A phone docks its tab bar under the content, as ClientShell does.
  const phone = density === "touch";
  return (
    <Screen density={density} footer={phone ? navigation : undefined} gap="$section">
      <Surface flex={1} justifyContent="center" padding="$section">
        <Text density={density} textRole="title">
          {clientDestinationLabel(active)}
        </Text>
      </Surface>
      {phone ? null : navigation}
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Client Navigation",
  component: NativeNavigationWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeNavigationWorkshop>;

type Story = StoryObj<typeof meta>;
// The device's own bar: Material on Android, the tab bar on iPhone.
const Touch: Story = {};
const TabBar: Story = { args: { variant: "tabBar" } };
const Material: Story = { args: { variant: "material" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Material, TabBar, Touch, Tv };
