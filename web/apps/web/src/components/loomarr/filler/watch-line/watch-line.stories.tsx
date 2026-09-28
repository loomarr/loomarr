import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { WatchLine } from "./watch-line";

// The Filler page header's live status — the web mock's `watchLine` (#1659): a 7px dot and a mono
// line under the page description.
//
// ⚠ **The dot's colour is the server's verdict** (`GET /v1/filler/watch`), never unconditionally
// green: a healthy pulse on an install with every source switched off would hide the one thing an
// operator most needs to see. These stories are its states.
const meta = {
  title: "Filler/WatchLine",
  component: WatchLine,
  decorators: [widthFrame(420)],
} satisfies Meta<typeof WatchLine>;

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
