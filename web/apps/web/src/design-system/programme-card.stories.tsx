import { semanticColors } from "@loomarr/design-system";
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
    (Story) => (
      <div style={{ boxSizing: "border-box", minHeight: "100vh", padding: 48 }}>
        <Story />
      </div>
    ),
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
  globals: { theme: "light" },
};

// The web mock's two layouts of the same card (#1659), at the widths the mock lays them out.
const Stacked: Story = {
  args: { artwork: <Artwork />, layout: "stacked", programme: classicEpisode },
  decorators: [
    (Story) => (
      <div style={{ width: 260 }}>
        <Story />
      </div>
    ),
  ],
};
const StackedMissingArtwork: Story = {
  args: { layout: "stacked", programme: missingArtworkEpisode },
  decorators: Stacked.decorators,
};
const Overlay: Story = {
  args: {
    artwork: <Artwork />,
    layout: "overlay",
    programme: classicEpisode,
    viewer: { device: "living room TV", initials: "HM", name: "A household member" },
  },
  decorators: [
    (Story) => (
      <div style={{ width: 352 }}>
        <Story />
      </div>
    ),
  ],
};
const OverlayOwnCard: Story = {
  args: { ...Overlay.args, highlighted: true, viewer: { device: "laptop", initials: "YO", name: "You" } },
  decorators: Overlay.decorators,
};
const OverlayMissingArtwork: Story = {
  args: { ...Overlay.args, artwork: undefined, programme: missingArtworkEpisode },
  decorators: Overlay.decorators,
};

export default meta;
export {
  LightFocused,
  MissingArtwork,
  Overlay,
  OverlayMissingArtwork,
  OverlayOwnCard,
  Pointer,
  Stacked,
  StackedMissingArtwork,
  TouchFocused,
  TvFocused,
};
