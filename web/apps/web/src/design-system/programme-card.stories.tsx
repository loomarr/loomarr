import { LoomarrProvider, semanticColors, semanticThemes } from "@loomarr/design-system";
import { classicEpisode, missingArtworkEpisode } from "@loomarr/fixtures";
import { ProgrammeCard } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

const Artwork = () => (
  <div
    style={{
      alignItems: "end",
      background: `linear-gradient(130deg, ${semanticColors.surface.elevated}, ${semanticColors.state.info})`,
      display: "flex",
      height: "100%",
      padding: 20,
      width: "100%",
    }}
  >
    <span style={{ color: semanticColors.content.primary, fontSize: 20, fontWeight: 700 }}>SPRINGFIELD</span>
  </div>
);

const meta = {
  title: "Loomarr Components/Programme Card",
  component: ProgrammeCard,
  decorators: [
    (Story, context) => {
      const theme = context.parameters.loomarrTheme === "light" ? "light" : "dark";
      return (
        <LoomarrProvider theme={theme}>
          <div style={{ background: semanticThemes[theme].surface.canvas, minHeight: "100vh", padding: 48 }}>
            <Story />
          </div>
        </LoomarrProvider>
      );
    },
  ],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ProgrammeCard>;

type Story = StoryObj<typeof meta>;

const Pointer: Story = { args: { artwork: <Artwork />, density: "pointer", programme: classicEpisode } };
const TouchFocused: Story = {
  args: { artwork: <Artwork />, density: "touch", focused: true, programme: classicEpisode },
};
const TvFocused: Story = {
  args: { artwork: <Artwork />, density: "tv", focused: true, programme: classicEpisode },
};
const MissingArtwork: Story = { args: { density: "pointer", programme: missingArtworkEpisode } };
const LightFocused: Story = {
  args: { artwork: <Artwork />, density: "pointer", focused: true, programme: classicEpisode },
  parameters: { loomarrTheme: "light" },
};

export default meta;
export { LightFocused, MissingArtwork, Pointer, TouchFocused, TvFocused };
