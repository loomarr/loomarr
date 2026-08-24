import {
  Action,
  ChoiceGroup,
  type Density,
  Field,
  Screen,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const InteractionWorkshop = ({ density = "pointer" }: { density?: Density }) => {
  const [artwork, setArtwork] = useState(true);
  const [systemTheme, setSystemTheme] = useState(false);
  const [guideDensity, setGuideDensity] = useState<"comfortable" | "compact">("comfortable");

  return (
    <Screen density={density} gap="$section">
      <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
        <Text density={density} textRole="metadata" tone="info">
          INTERACTION CONTRACT
        </Text>
        <Text density={density} textRole="display">
          Controls that behave the same everywhere.
        </Text>
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Actions
        </Text>
        <Surface
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection={density === "touch" ? "column" : "row"}
          flexWrap="wrap"
          gap="$control"
        >
          <Action density={density} onPress={() => undefined}>
            Primary
          </Action>
          <Action density={density} onPress={() => undefined} tone="secondary">
            Keyboard focus
          </Action>
          <Action density={density} onPress={() => undefined} selected tone="secondary">
            Selected
          </Action>
          <Action density={density} disabled onPress={() => undefined}>
            Disabled
          </Action>
          <Action density={density} onPress={() => undefined} tone="danger">
            Disconnect
          </Action>
        </Surface>
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Fields
        </Text>
        <Field
          density={density}
          description="Use the address shown by your Loomarr server."
          label="Server address"
          onChangeText={() => undefined}
          placeholder="https://loomarr.media"
          value=""
        />
        <Field
          density={density}
          error="Enter an HTTPS address."
          label="Invalid address"
          onChangeText={() => undefined}
          value="http://loomarr.local"
        />
        <Field density={density} disabled label="Managed server" value="https://living-room.local" />
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Toggles and choices
        </Text>
        <Toggle
          checked={artwork}
          density={density}
          description="Show programme artwork in Guide details."
          label="Episode artwork"
          onCheckedChange={setArtwork}
        />
        <Toggle
          checked={systemTheme}
          density={density}
          kind="switch"
          label="Follow system theme"
          onCheckedChange={setSystemTheme}
        />
        <Toggle
          checked={false}
          density={density}
          disabled
          kind="switch"
          label="Unavailable setting"
          onCheckedChange={() => undefined}
        />
        <ChoiceGroup
          density={density}
          label="Guide density"
          onValueChange={setGuideDensity}
          options={[
            { description: "More programme detail.", label: "Comfortable", value: "comfortable" },
            { description: "More channels at once.", label: "Compact", value: "compact" },
          ]}
          value={guideDensity}
        />
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Foundations/Interaction",
  component: InteractionWorkshop,
  args: { density: "pointer" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof InteractionWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Keyboard focus" }).focus();
  },
};

export default meta;
export { Focused, Light, Pointer, Touch, Tv };
