import { Stack } from "@loomarr/design-system";
import { ChannelIdent } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// A channel's mark in the guide rail and the channel header. A logo when one exists, otherwise a
// monogram in the hue of the channel number, so renaming a channel does not change how it looks.
const meta = {
  title: "Loomarr Components/Channel Ident",
  component: ChannelIdent,
  args: { name: "1980s Action Heroes", number: 3 },
} satisfies Meta<typeof ChannelIdent>;

type Story = StoryObj<typeof meta>;

const Monogram: Story = {};

// A leading number is skipped: "90s Action Heroes" reads better as "AH" than as "9A".
const NumericName: Story = { args: { name: "90s Action Heroes", number: 42 } };

const SingleWord: Story = { args: { name: "Cartoons", number: 7 } };

// Different numbers cycle the palette, so adjacent rows stay distinguishable.
const PaletteSpread: Story = {
  render: () => (
    <Stack direction="row" gap="2">
      {[
        ["Sparrowmoor Classics", 1],
        ["Star Transit Classics", 2],
        ["1980s Action Heroes", 3],
        ["Saturday Morning Kids", 4],
        ["Late Night Sci-Fi", 5],
      ].map(([name, number]) => (
        <ChannelIdent key={number} name={name as string} number={number as number} />
      ))}
    </Stack>
  ),
};

// An uploaded icon replaces the monogram. Inline data URI, never a remote URL: a remote image
// bypasses the visual suite's stubbed fetch and races the snapshot.
const WithLogo: Story = {
  args: {
    logo:
      "data:image/svg+xml;base64," +
      btoa(
        '<svg xmlns="http://www.w3.org/2000/svg" width="60" height="60">' +
          '<rect width="60" height="60" fill="#4CC9E8"/>' +
          '<circle cx="30" cy="30" r="16" fill="#0B0C0E"/></svg>',
      ),
  },
};

const Large: Story = { args: { size: 48 } };

export default meta;
export { Large, Monogram, NumericName, PaletteSpread, SingleWord, WithLogo };
