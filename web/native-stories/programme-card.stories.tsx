import { Surface, Text } from "@loomarr/design-system";
import { classicEpisode, missingArtworkEpisode } from "@loomarr/fixtures";
import { ProgrammeCard } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const Artwork = () => (
  <Surface
    alignItems="center"
    backgroundColor="$surfaceElevated"
    borderWidth={0}
    flex={1}
    justifyContent="center"
    width="100%"
  >
    <Text textRole="label">PROGRAMME ARTWORK</Text>
  </Surface>
);

const meta = {
  title: "Loomarr/Programme Card",
  component: ProgrammeCard,
  args: { artwork: <Artwork />, density: "touch", focused: false, programme: classicEpisode },
} satisfies Meta<typeof ProgrammeCard>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const TvFocused: Story = { args: { density: "tv", focused: true } };
const MissingArtwork: Story = { args: { artwork: undefined, programme: missingArtworkEpisode } };

export default meta;
export { MissingArtwork, Touch, TvFocused };
