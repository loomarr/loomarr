import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ActivityFeed } from "./activity-feed";

// A fixed clock, so the relative times do not churn the visual baseline on every run.
const NOW = Date.parse("2026-07-29T21:50:00Z");
const ago = (mins: number) => Math.floor((NOW - mins * 60_000) / 1000);

const meta = {
  title: "Dashboard/ActivityFeed",
  component: ActivityFeed,
  args: {
    now: NOW,
    entries: [
      {
        id: "a1",
        at: ago(2),
        kind: "title",
        level: "info",
        text: "Darkwing Duck landed — ready to schedule",
      },
      {
        id: "a2",
        at: ago(9),
        kind: "proposal",
        level: "info",
        text: "Request approved — 3 titles queued for download",
      },
      { id: "a3", at: ago(14), kind: "filler", level: "info", text: "Filler catalog synced — 9 clips" },
      { id: "a4", at: ago(20), kind: "system", level: "warn", text: "Seerr timed out — retrying" },
      { id: "a5", at: ago(38), kind: "channel", level: "error", text: "CH 12 could not reach Tunarr" },
    ],
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof ActivityFeed>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A fresh install has done nothing yet — that is an explanation, not an empty box.
const Empty: Story = {
  args: { entries: [] },
};

export default meta;
export { Default, Empty };
