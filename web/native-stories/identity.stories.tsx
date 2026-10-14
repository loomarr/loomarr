import { Screen, Surface, Text } from "@loomarr/design-system";
import { classicEpisode } from "@loomarr/fixtures";
import { ChannelIdentity, ProgrammeIdentity } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const NativeIdentityWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density} gap="$section">
    <ChannelIdentity
      channel={{
        channelLogoState: "missing",
        channelName: classicEpisode.channelName,
        channelNumber: classicEpisode.channelNumber,
      }}
      density={density}
    />
    <Surface padding="$section">
      <ProgrammeIdentity
        density={density}
        programme={{
          badge: classicEpisode.badge,
          description: classicEpisode.description,
          episodeLabel: classicEpisode.episodeLabel,
          seriesTitle: classicEpisode.seriesTitle,
          timeLabel: classicEpisode.timeLabel,
          title: classicEpisode.title,
        }}
      />
    </Surface>
    <Text density={density} textRole="metadata">
      The same missing-logo fallback and metadata hierarchy render on every host.
    </Text>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Media Identity",
  component: NativeIdentityWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeIdentityWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
