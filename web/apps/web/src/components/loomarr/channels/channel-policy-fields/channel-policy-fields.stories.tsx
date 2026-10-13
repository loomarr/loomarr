import type { ChannelPolicy } from "@loomarr/api";
import { disjointProgrammingDatesPolicy } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { eraDates } from "@/lib/era-dates";
import { widthFrame } from "@/test/story-utils";
import { ChannelPolicyFields } from "./channel-policy-fields";

const noop = () => {};

// Each field's help is now an (i) FieldHelp tooltip, which needs a TooltipProvider ancestor
// (mounted at the app root in the real app; supplied here for isolation).
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

// A channel's ChannelPolicy as plain-language editable fields (programming-design §8).
const meta = {
  title: "Channels/ChannelPolicyFields",
  component: ChannelPolicyFields,
  args: { onChange: noop },
  decorators: [withTooltip, widthFrame(480)],
} satisfies Meta<typeof ChannelPolicyFields>;

type Story = StoryObj<typeof meta>;

const Empty: Story = { args: { policy: {} } };

const populated: ChannelPolicy = {
  ordering: "shuffle",
  audience: { ceiling: "TV-14" },
  scope: { dates: eraDates({ from: 1990, to: 1999 }) },
  separation: { movieNoRepeat: "168h", episodeNoRepeat: "24h" },
};

const Populated: Story = { args: { policy: populated } };

// The safety ceiling set to the strictest value, to check the caption reads correctly
// alongside a real restriction rather than only against "No limit".
const StrictCeiling: Story = {
  args: { policy: { audience: { ceiling: "TV-Y" } } },
};

// Per-axis dates an era cannot express: the dates field shows the per-axis editor.
const DisjointProgrammingDates: Story = {
  args: { policy: disjointProgrammingDatesPolicy },
};

// Every field a channel override: each badge reads Channel override and each Reset is live.
const AllOverridden: Story = {
  args: {
    policy: {
      ordering: "sequential",
      audience: { ceiling: "TV-PG", unrated: "exclude" },
      scope: { dates: eraDates({ from: 1990, to: 1999 }), runtimeMax: 90 * 60 },
      separation: { movieNoRepeat: "168h", episodeNoRepeat: "24h", seriesMinGap: "2h", blockMax: 2 },
    },
  },
};

// The operator opened the per-axis editor from an era: the era fills all three axes, and
// "Use a single era" is offered because going back loses nothing.
const SeparateWindows: Story = {
  args: { policy: populated, show: "scope" },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Use separate date windows" }));
    await canvas.findByRole("button", { name: "Use a single era" });
  },
};

export default meta;
export { AllOverridden, DisjointProgrammingDates, Empty, Populated, SeparateWindows, StrictCeiling };
