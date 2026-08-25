import { AdaptiveSplit, Disclosure, Screen, ScrollFrame, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const NativeLayoutWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [expanded, setExpanded] = useState(true);
  return (
    <Screen density={density}>
      <ScrollFrame density={density}>
        <Text density={density} textRole="title">
          Shared responsive composition
        </Text>
        <AdaptiveSplit
          density={density}
          primary={
            <Surface gap="$control" padding="$section">
              <Text density={density} textRole="metadata" tone="live">
                NOW PLAYING
              </Text>
              <Text density={density} textRole="title">
                Marge vs. the Monorail
              </Text>
            </Surface>
          }
          secondary={
            <Disclosure
              density={density}
              expanded={expanded}
              label="Episode information"
              onExpandedChange={setExpanded}
            >
              <Text density={density} textRole="body">
                Season 4 · Episode 12 · January 14, 1993
              </Text>
            </Disclosure>
          }
        />
      </ScrollFrame>
    </Screen>
  );
};

const meta = {
  title: "Loomarr/Layout",
  component: NativeLayoutWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeLayoutWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
