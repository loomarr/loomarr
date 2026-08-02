import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PoolHealth } from "./pool-health";

// Catalog-wide filler health — the strip above every tab on the Filler page (V35).
//
// It is a strip rather than a tab because catalog health is the context the other tabs are read
// in. Every per-channel number is the SAME answer the channel page's coverage meter gives: the
// server computes it by calling `Coverage` once per live channel, so there is no aggregate
// ladder to drift from the real one.
const meta = {
  title: "Filler/PoolHealth",
  component: PoolHealth,
  decorators: [widthFrame(900)],
} satisfies Meta<typeof PoolHealth>;

type Story = StoryObj<typeof meta>;

// The healthy case: everything tagged, every channel matching exactly, so no diagnosis callout.
const Healthy: Story = {
  args: {
    pool: {
      clips: 412,
      commercials: 380,
      eligible: 374,
      untagged: 0,
      channels: [
        { channelId: "ch-42", name: "Saturday Mornings", number: 42, level: "exact", total: 88 },
        { channelId: "ch-7", name: "Late Night Sci-Fi", number: 7, level: "exact", total: 61 },
      ],
    },
    onProposePull: () => {},
  },
};

// The interesting case, and the one this component exists for: a channel whose breaks fall all
// the way through to the built-in card, named so an operator can act on it.
const ChannelWithNothingToPlay: Story = {
  args: {
    pool: {
      clips: 120,
      commercials: 90,
      eligible: 61,
      untagged: 14,
      channels: [
        { channelId: "ch-3", name: "Newsreel", number: 3, level: "bumper_card", total: 0 },
        { channelId: "ch-42", name: "Saturday Mornings", number: 42, level: "exact", total: 44 },
      ],
    },
    onProposePull: () => {},
  },
};

// ⚠ The number that surprises people: a catalog of compilations reads as healthy by clip count
// and can fill nothing, because none of it is short enough for a break. Shown as a warning so it
// is not mistaken for a large healthy catalog.
const NothingFitsABreak: Story = {
  args: {
    pool: {
      clips: 500,
      commercials: 500,
      eligible: 0,
      untagged: 500,
      channels: [{ channelId: "ch-3", name: "Newsreel", number: 3, level: "bumper_card", total: 0 }],
    },
    onProposePull: () => {},
  },
};

// A fresh install: nothing downloaded, no channels yet. This is the state the pull button exists
// for, and the channel stats are omitted entirely rather than claiming "0/0 covered".
const FreshInstall: Story = {
  args: {
    pool: { clips: 0, commercials: 0, eligible: 0, untagged: 0, channels: [] },
    onProposePull: () => {},
  },
};

// A member reads catalog health but cannot start an acquisition, so no button.
const ReadOnly: Story = {
  args: { pool: Healthy.args.pool },
};

export default meta;
export { ChannelWithNothingToPlay, FreshInstall, Healthy, NothingFitsABreak, ReadOnly };
