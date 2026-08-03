import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { WatchPill } from "./watch-pill";

// The Filler page header's live status — the mock's `watchLine` (§10 V38c).
//
// The card is the mock's, measured off the rendered prototype: `bg-card` on a 1px border, 8px
// radius, 8/13px padding, a 9px gap, a 7px dot and 11px mono text.
//
// ⚠ **The DOT is an addition the mock does not specify.** The prototype draws it unconditionally
// green because a static mock has no failure states. Shipping that verbatim would mean a healthy
// green pulse on an install with every source switched off — the one moment an operator most
// needs to be told otherwise. These three stories are the states that replaced it, and the
// verdict comes from `GET /v1/filler/watch` rather than from the client.
const meta = {
  title: "Filler/WatchPill",
  component: WatchPill,
  decorators: [widthFrame(420)],
} satisfies Meta<typeof WatchPill>;

type Story = StoryObj<typeof meta>;

// Sources on, clips arriving. The only state that pulses — motion means "running here".
const Healthy: Story = {
  args: { status: "4 of 5 sources on · 9 clips · last scan 2m ago", health: "healthy" },
};

// Configured, but nothing is scanning: every source switched off, or the catalog is empty, or
// everything that reports a fetch went quiet days ago.
const NeedsAttention: Story = {
  args: { status: "0 of 5 sources on · 9 clips", health: "attention" },
};

// ⚠ A fresh install, NOT a fault. An amber warning on first boot would read as a problem the
// operator caused, when the truth is simply that there is work still to do.
const Unconfigured: Story = {
  args: { status: "0 of 0 sources on · 0 clips", health: "unconfigured" },
};

// ⚠ **The state a first fetch actually lands in, and the one this pill got WRONG.** Auto-fetch
// holds everything it downloads for review, so a successful first pull leaves the catalog at zero
// and the Incoming queue full. The header read "5 of 5 sources on · 0 clips" — a working fetcher
// rendered as a broken one. The two counts stay separate clauses: summing them would claim a
// channel can play clips nobody has approved.
//
// Healthy, deliberately. Nothing is wrong here; there is just something to review.
const Holding: Story = {
  args: { status: "5 of 5 sources on · 0 clips · 12 waiting · last scan 1m ago", health: "healthy" },
};

export default meta;
export { Healthy, Holding, NeedsAttention, Unconfigured };
