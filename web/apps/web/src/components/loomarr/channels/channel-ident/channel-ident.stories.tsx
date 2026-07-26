import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChannelIdent } from "./channel-ident";

// A channel's mark in the guide rail (§12). A logo when one exists, otherwise a derived
// monogram — because a rail of unlabelled rows is far harder to scan than one with even a
// crude mark, and most channels never get a logo uploaded.
const meta = {
  title: "Channels/ChannelIdent",
  component: ChannelIdent,
  args: { name: "1980s Action Heroes", number: 3 },
} satisfies Meta<typeof ChannelIdent>;

type Story = StoryObj<typeof meta>;

// Initials of the first two significant words, hatched, in a colour keyed to the channel
// NUMBER — so renaming a channel does not change how the rail looks.
const Monogram: Story = {};

// A leading number is skipped: "90s Action Heroes" reads better as "AH" than as "9A".
const NumericName: Story = { args: { name: "90s Action Heroes", number: 42 } };

// One word falls back to its first two letters.
const SingleWord: Story = { args: { name: "Cartoons", number: 7 } };

// Different numbers cycle the palette, so adjacent rows stay distinguishable.
const PaletteSpread: Story = {
  render: () => (
    <div className="flex gap-2">
      {[
        ["Springfield Classics", 1],
        ["Star Trek Classics", 2],
        ["1980s Action Heroes", 3],
        ["Saturday Morning Kids", 4],
        ["Late Night Sci-Fi", 5],
      ].map(([name, number]) => (
        <ChannelIdent key={number} name={name as string} number={number as number} />
      ))}
    </div>
  ),
};

// An uploaded icon replaces the monogram entirely. Inline data URI, never a remote URL: a
// remote image bypasses the visual suite's stubbed fetch and races the snapshot.
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

// The rail scales with zoom, so the mark must too.
const Large: Story = { args: { size: 48 } };

export default meta;
export { Large, Monogram, NumericName, PaletteSpread, SingleWord, WithLogo };
