import { Screen, Surface, Text } from "@loomarr/design-system";
import { ChannelIdentity, ProgrammeIdentity } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const NativeIdentityWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density} gap="$section">
    <ChannelIdentity
      channel={{ channelLogoState: "missing", channelName: "Classic Animation", channelNumber: "07" }}
      density={density}
    />
    <Surface padding="$section">
      <ProgrammeIdentity
        density={density}
        programme={{
          badge: { label: "On now", tone: "live" },
          description: "Springfield considers an ambitious monorail proposal.",
          episodeLabel: "S04E12",
          seriesTitle: "The Simpsons",
          timeLabel: "7:00–7:30 PM",
          title: "Marge vs. the Monorail",
        }}
      />
    </Surface>
    <Text density={density} textRole="metadata">
      The same missing-logo fallback and metadata hierarchy render on every host.
    </Text>
  </Screen>
);

const meta = {
  title: "Loomarr/Media Identity",
  component: NativeIdentityWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeIdentityWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
