import type { ChannelPolicy } from "@loomarr/api";
import { ruleVocabularyFixture } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ChannelRulesEditor } from "./channel-rules-editor";

const noop = () => {};

// Each row's remove button carries a tooltip (Radix), which needs a TooltipProvider
// ancestor — mounted at the app root in the real app, supplied here for isolation.
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

// ChannelRulesEditor — the "Programming rules" chip list (programming-design §6.5/§6.6):
// token-based WHEN/WHAT/HOW rows, reorderable by drag, priority derived from list order.
const meta = {
  title: "Channels/ChannelRulesEditor",
  component: ChannelRulesEditor,
  args: { onChange: noop, vocabulary: ruleVocabularyFixture },
  decorators: [withTooltip, widthFrame(680)],
} satisfies Meta<typeof ChannelRulesEditor>;

type Story = StoryObj<typeof meta>;

// No rules yet — the explainer + Add affordance, not an error.
const Empty: Story = { args: { policy: {} } };

// A populated set showing the range: a holiday marathon (top, wins overlaps), a
// weekend daypart scoped to kids genres, and a plain weekday syndication rule.
const POPULATED: ChannelPolicy = {
  rules: [
    {
      id: "r1",
      label: "Christmas · Marathon",
      priority: 30,
      when: { holiday: "christmas" },
      how: { ordering: "sequential", noBreaks: true, separation: { blockMax: -1 } },
      window: "0s",
    },
    {
      id: "r2",
      label: "Weekend · Kids-safe genres",
      priority: 20,
      when: { weekend: true },
      what: { genres: { include: ["Animation", "Family", "Kids"] } },
    },
    {
      id: "r3",
      label: "Weekday · Syndication",
      priority: 10,
      when: { weekday: true },
      how: { ordering: "syndication" },
    },
  ],
};

const Populated: Story = {
  args: {
    policy: POPULATED,
    lineupKeys: [
      { key: "series:tvdb:81189", title: "Breaking Bad" },
      { key: "movie:tmdb:106", title: "Predator" },
    ],
  },
};

// Adding a rule: play() clicks Add and shows the new row at the top with its default
// tokens — proving priority-by-position (a fresh rule always outranks nothing yet, and
// stacks above whatever is already there).
const AddingARule: Story = {
  args: {
    policy: {
      rules: [{ id: "r3", label: "Weekday · Syndication", priority: 10, when: { weekday: true } }],
    },
  },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /add rule/i }));
  },
};

export default meta;
export { AddingARule, Empty, Populated };
