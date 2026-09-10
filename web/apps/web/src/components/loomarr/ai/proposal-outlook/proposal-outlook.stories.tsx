import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { outlook } from "@/test/fixtures/outlook";
import { ProposalOutlook } from "./proposal-outlook";

const meta = {
  title: "AI/ProposalOutlook",
  component: ProposalOutlook,
  decorators: [
    (Story) => (
      <div className="w-full max-w-xl p-4">
        <Story />
      </div>
    ),
  ],
  args: { assessment: outlook(), onAddVariety: fn() },
} satisfies Meta<typeof ProposalOutlook>;

type Story = StoryObj<typeof meta>;
const Healthy: Story = {};
const Waiting: Story = {
  args: {
    assessment: outlook({
      state: "waiting",
      titles: 2,
      scheduledTitles: 0,
      missingAcquisitions: 2,
      mix: { core: 1, adjacent: 1, discovery: 0, unknown: 0 },
      programs: 0,
      seasons: 0,
      uniqueRuntimeMs: 0,
      firstRepeatMs: null,
    }),
  },
};
const Thin: Story = {
  args: {
    assessment: outlook({
      titles: 1,
      scheduledTitles: 1,
      programs: 1,
      seasons: 0,
      uniqueRuntimeMs: 90 * 60_000,
      firstRepeatMs: 90 * 60_000,
      thin: true,
      mix: { core: 1, adjacent: 0, discovery: 0, unknown: 0 },
    }),
  },
};
const Uncertain: Story = {
  args: {
    assessment: outlook({
      state: "uncertain",
      scheduledTitles: 0,
      unknownTitles: 3,
      programs: 0,
      seasons: 0,
      uniqueRuntimeMs: 0,
      firstRepeatMs: null,
      mix: { core: 0, adjacent: 0, discovery: 0, unknown: 3 },
    }),
  },
};

export default meta;
export { Healthy, Thin, Uncertain, Waiting };
