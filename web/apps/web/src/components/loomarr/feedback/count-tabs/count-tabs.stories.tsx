import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { CountTabs } from "./count-tabs";

const noop = () => {};

// The v2 mock's Queue tab bar: an underlined tab per section, each carrying a count pill. The
// count is the reason the bar exists — an admin's first question is "how many need me?".
const meta = {
  title: "Feedback/CountTabs",
  component: CountTabs,
  args: {
    label: "Queue sections",
    onSelect: noop,
    tabs: [
      { id: "approval", label: "Needs approval", count: 3 },
      { id: "flight", label: "In flight", count: 12 },
      { id: "history", label: "History", count: 14 },
    ],
  },
  // The active tab sets `aria-controls="panel-<id>"`, so the story has to MOUNT that panel —
  // otherwise the reference dangles and axe fails `aria-valid-attr-value` at serious impact. A
  // tablist without its panel is also just an incomplete widget: this is the shape the Queue
  // route actually renders, not scaffolding added to satisfy the linter.
  decorators: [
    (Story, ctx) => {
      const active = (ctx.args as { activeId?: string }).activeId ?? "";
      return (
        <div>
          <Story />
          <div
            id={`panel-${active}`}
            role="tabpanel"
            aria-labelledby={`tab-${active}`}
            className="p-4 text-muted-foreground text-sm"
          >
            Panel content
          </div>
        </div>
      );
    },
    widthFrame(560),
  ],
} satisfies Meta<typeof CountTabs>;

type Story = StoryObj<typeof meta>;

const NeedsApproval: Story = { args: { activeId: "approval" } };

const History: Story = { args: { activeId: "history" } };

// A quiet queue: nothing pending, so the count pills read zero rather than disappearing —
// "0 need you" is information, an absent badge is ambiguous.
const Empty: Story = {
  args: {
    activeId: "flight",
    tabs: [
      { id: "approval", label: "Needs approval", count: 0 },
      { id: "flight", label: "In flight", count: 0 },
      { id: "history", label: "History", count: 0 },
    ],
  },
};

export default meta;
export { Empty, History, NeedsApproval };
