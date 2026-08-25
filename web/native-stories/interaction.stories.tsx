import { Action, ChoiceGroup, Field, Screen, Surface, Text, Toggle } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";
import { ScrollView } from "react-native";

const NativeInteractionWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [artwork, setArtwork] = useState(true);
  const [guideDensity, setGuideDensity] = useState<"comfortable" | "compact">("comfortable");
  return (
    <Screen density={density}>
      <ScrollView contentContainerStyle={{ gap: density === "tv" ? 28 : 18 }}>
        <Text density={density} textRole="title">
          Shared interaction states
        </Text>
        <Surface gap="$control" padding="$section">
          <Action density={density} onPress={() => undefined}>
            Primary
          </Action>
          <Action density={density} selected tone="secondary" onPress={() => undefined}>
            Selected
          </Action>
          <Action density={density} disabled onPress={() => undefined}>
            Disabled
          </Action>
        </Surface>
        <Surface gap="$control" padding="$section">
          <Field density={density} label="Server address" placeholder="https://loomarr.media" />
          <Field density={density} error="Enter an HTTPS address." label="Invalid address" value="http://" />
          <Toggle checked={artwork} density={density} label="Episode artwork" onCheckedChange={setArtwork} />
          <ChoiceGroup
            density={density}
            label="Guide density"
            onValueChange={setGuideDensity}
            options={[
              { label: "Comfortable", value: "comfortable" },
              { label: "Compact", value: "compact" },
            ]}
            value={guideDensity}
          />
        </Surface>
      </ScrollView>
    </Screen>
  );
};

const meta = {
  title: "Loomarr/Interaction",
  component: NativeInteractionWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeInteractionWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
