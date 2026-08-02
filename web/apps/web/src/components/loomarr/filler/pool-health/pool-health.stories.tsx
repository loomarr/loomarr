import { emptyPool, healthyPool, thinPool, unplaceablePool } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PoolHealth } from "./pool-health";

// Catalog-wide filler health — the strip above every tab on the Filler page (V35).
//
// It is a strip rather than a tab because catalog health is the context the other tabs are read
// in. Every per-channel number is the SAME answer the channel page's coverage meter gives: the
// server computes it by calling `Coverage` once per live channel, so there is no aggregate
// ladder to drift from the real one.
//
// Args come from `@loomarr/fixtures` per frontend-design §5.1b — a story that hand-rolls domain
// data is a web-only literal the native Storybook cannot reuse.
const meta = {
  title: "Filler/PoolHealth",
  component: PoolHealth,
  decorators: [widthFrame(900)],
} satisfies Meta<typeof PoolHealth>;

type Story = StoryObj<typeof meta>;

// Everything tagged, every channel matching exactly, so no diagnosis callout.
const Healthy: Story = {
  args: { pool: healthyPool, onProposePull: () => {} },
};

// The case this component exists for: a channel whose breaks fall all the way through to the
// built-in card, named so an operator can act on it.
const ChannelWithNothingToPlay: Story = {
  args: { pool: thinPool, onProposePull: () => {} },
};

// ⚠ The number that surprises people: a catalog of compilations reads as healthy by clip count
// and can fill nothing, because none of it is short enough for a break.
const NothingFitsABreak: Story = {
  args: { pool: unplaceablePool, onProposePull: () => {} },
};

// A fresh install: nothing downloaded, no channels yet. This is the state the pull button exists
// for, and the channel stats are omitted entirely rather than claiming "0/0 covered".
const FreshInstall: Story = {
  args: { pool: emptyPool, onProposePull: () => {} },
};

// A member reads catalog health but cannot start an acquisition, so no button.
const ReadOnly: Story = {
  args: { pool: healthyPool },
};

export default meta;
export { ChannelWithNothingToPlay, FreshInstall, Healthy, NothingFitsABreak, ReadOnly };
